package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Exercise the public CLI in a separate process, including argument errors.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("GNETCLI_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			break
		}
	}
	main()
	os.Exit(0) // Do not mix the test runner output with the CLI JSON.
}

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		name, contents, password, command string
		exitCode, commandStatus           int
		usePasswordFlag                   bool
	}{
		{"password_lf", " secret \n", " secret ", "show clock", 0, 0, false},
		{"password_crlf", " secret \r\n", " secret ", "show clock", 0, 0, false},
		{"password_no_newline", " secret ", " secret ", "show clock", 0, 0, false},
		{"password_lone_cr", "secret\r", "secret\r", "show clock", 0, 0, false},
		{"password_two_newlines", "secret\n\n", "secret\n", "show clock", 0, 0, false},
		{"password_utf8", " séc☃ \n", " séc☃ ", "show clock", 0, 0, false},
		{"device_error_keeps_exit_code", " secret \n", " secret ", "invalid\nshow clock", 0, 1, false},
		{"wrong_password", "wrong\n", " secret ", "show clock", 2, 0, false},
		{"existing_password_flag", "", " secret ", "show clock", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed := make(chan struct{})
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { listener.Close() })
			_, key, err := ed25519.GenerateKey(rand.Reader)
			require.NoError(t, err)
			signer, err := ssh.NewSignerFromKey(key)
			require.NoError(t, err)
			config := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
				if c.User() == "test" && string(pass) == tc.password {
					return nil, nil
				}
				return nil, fmt.Errorf("authentication rejected")
			}}
			config.AddHostKey(signer)
			go func() {
				defer close(closed)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(15 * time.Second))
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for ch := range channels {
					channel, reqs, err := ch.Accept()
					if err != nil {
						return
					}
					go func() {
						for req := range reqs {
							req.Reply(true, nil)
							if req.Type == "shell" {
								channel.Write([]byte("switch#"))
							}
						}
					}()
					reader := bufio.NewReader(channel)
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							break
						}
						cmd := strings.TrimSpace(line)
						output := ""
						switch cmd {
						case "show clock":
							output = "12:00:00\r\n"
						case "invalid":
							output = "% Invalid input\r\n"
						}
						channel.Write([]byte(cmd + "\r\n" + output + "switch#"))
					}
					channel.Close()
				}
			}()
			passwordFile := filepath.Join(t.TempDir(), "password")
			require.NoError(t, os.WriteFile(passwordFile, []byte(tc.contents), 0600))
			host, port, err := net.SplitHostPort(listener.Addr().String())
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := []string{"-test.run=^TestCLIProcess$", "--",
				"-hostname", host, "-port", port, "-devtype", "arista", "-login", "test",
				"-command", tc.command, "-json"}
			if tc.usePasswordFlag {
				args = append(args, "-password", tc.password)
			} else {
				args = append(args, "-password-file", passwordFile)
			}
			process := osexec.CommandContext(ctx, os.Args[0], args...)
			process.Env = append(os.Environ(), "GNETCLI_TEST_PROCESS=1", "SSH_AUTH_SOCK=")
			var stdout, stderr bytes.Buffer
			process.Stdout, process.Stderr = &stdout, &stderr
			err = process.Run()
			if tc.exitCode == 0 {
				require.NoError(t, err, stderr.String())
			} else {
				var exit *osexec.ExitError
				require.ErrorAs(t, err, &exit)
				require.Equal(t, tc.exitCode, exit.ExitCode(), stderr.String())
			}
			require.NotContains(t, stdout.String()+stderr.String(), tc.password)
			if tc.exitCode != 2 {
				var results []struct {
					Status int
					Output string
				}
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &results))
				require.Equal(t, tc.commandStatus, results[0].Status)
				require.Len(t, results, len(strings.Split(tc.command, "\n")))
				require.Equal(t, 0, results[len(results)-1].Status)
				require.Contains(t, results[len(results)-1].Output, "12:00:00")
			}
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("connection was not closed")
			}
		})
	}
}

func TestCLIInvalidPasswordArguments(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid-utf8")
	require.NoError(t, os.WriteFile(invalid, []byte("FILE_CONTENT_MUST_NOT_BE_LOGGED\xff"), 0600))
	missing := filepath.Join(t.TempDir(), "missing")
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"invalid_utf8", []string{"-password-file", invalid}, "password file is not valid UTF-8"},
		{"missing_file", []string{"-password-file", missing}, "unable to read password file"},
		{"empty_path", []string{"-password-file", ""}, "unable to read password file"},
		{"directory", []string{"-password-file", t.TempDir()}, "unable to read password file"},
		{"both_flags", []string{"-password", "ARGUMENT_MUST_NOT_BE_LOGGED", "-password-file", invalid}, "mutually exclusive"},
		{"empty_password_flag", []string{"-password", "", "-password-file", invalid}, "mutually exclusive"},
		{"reversed_flags", []string{"-password-file", invalid, "-password", "ARGUMENT_MUST_NOT_BE_LOGGED"}, "mutually exclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			process := osexec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, tc.args...)...)
			process.Env = append(os.Environ(), "GNETCLI_TEST_PROCESS=1")
			output, err := process.CombinedOutput()
			var exit *osexec.ExitError
			require.ErrorAs(t, err, &exit, string(output))
			require.Equal(t, 2, exit.ExitCode())
			require.Contains(t, string(output), tc.message)
			require.NotContains(t, string(output), "FILE_CONTENT_MUST_NOT_BE_LOGGED")
			require.NotContains(t, string(output), "ARGUMENT_MUST_NOT_BE_LOGGED")
		})
	}
}
