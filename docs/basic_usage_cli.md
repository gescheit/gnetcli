Gnetcli 'cli' is a tool for executing commands on a device.
It is useful for using in automation and device regular expression debuging.

For example:

```shell
cli -hostname myhost -devtype huawei -command $'dis clock\ndis ver0' -password $password -json
```

```json
[
  {
    "output": "2023-10-29 10:00:00\nSunday\nTime Zone(UTC) : UTC\n",
    "error": "",
    "status": 0,
    "cmd": "dis clock"
  },
  {
    "output": "",
    "error": "              ^\nError: Unrecognized command found at '^' position.\n",
    "status": 1,
    "cmd": "dis ver0"
  }
]
```

### Help

```
Usage of cli:
  -command string
    	Command
  -debug
    	Set debug log level
  -dev-conf string
    	Path to yaml with device types
  -devtype string
    	Device type from dev-conf file or from predifined: juniper, huawei, h3c, arista, cisco, nxos, bcomos, pc, ros, netconf, aruos, eltex, asa, fortios, sitonica
  -hostname string
    	Hostname
  -json
    	Output in JSON
  -login string
    	Login
  -password string
    	Password
  -port int
    	Port (default 22)
  -use-ssh-config
      Use default ssh config ($HOME/.ssh/config, falling back to /etc/ssh/ssh_config) to search for options for provided hostname. Supported keywords: User, IdentityAgent, ForwardAgent, IdentityFile. If option is specified in config, it will override options from other sources (e.g. User will override -login if specified)
  -ssh-config-passphrase string
      Passphrase for IdentityFiles specified in ssh config.
```


### Password file

Use `-password-file` to keep the password itself out of process arguments:

```shell
cli -hostname switch.example.test -devtype arista -login mylogin \
  -password-file /run/secrets/device-password -command 'show clock' -json
```

The file must contain UTF-8 text. Exactly one final LF or CRLF is removed;
spaces, a lone CR, and any other preceding content are preserved. Protect the
file with restrictive permissions (for example, mode 600) and do not commit it.

`-password-file` and `-password` are mutually exclusive, including an explicitly
empty `-password`. An unreadable file, invalid UTF-8 or conflicting options
produces an error without printing the password and exits with code 2.
Existing command-error handling and process exit codes are unchanged.
