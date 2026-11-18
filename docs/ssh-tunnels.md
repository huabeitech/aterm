# SSH tunnel management

Local TCP forwarding connects an application on your computer to a service reachable from a saved SSH host. For example, configure `127.0.0.1:13306` through a bastion host to `db.internal:3306`, start the tunnel, and point a database client at `127.0.0.1:13306`.

## Install

Build or download the SSH tunnels `.aterm-plugin` for your platform and install it through **Settings → Plugins**. Existing tunnel configurations migrate automatically; installation is still required to use them. See [Native plugins](native-plugins.md).

## Configure and use

1. Select **Tunnels** in the bottom view bar of a saved SSH connection. Use the plugin entry in the sidebar footer for the global workspace, or the plugin action in host details.
2. Choose **New tunnel**. Enter a name and select a service preset. The selected SSH host is prefilled. Choose **This SSH host** for a local service, or **Another private host** to enter its address. Set the service and local ports; the listen address is under **Advanced**.
3. Choose **Save** for later use, or **Save and start**. Unlock protected data and complete any SSH authentication or host verification prompts.
4. Connect an external application to the displayed local address. **Copy local address** copies the address and port.
5. Choose **Stop** when finished. This closes both the listener and existing forwarded connections.

The target address is resolved and contacted from the selected SSH host. A target of `127.0.0.1` refers to that host itself. Saved SSH authentication, managed keys, and jump hosts are reused. Target service credentials remain in the external application.

The listen address defaults to `127.0.0.1`. Both IPv4 and IPv6 literals are accepted. A non-loopback listener may expose the forwarded service to other devices; the editor displays a notice. Local ports are fixed and never silently replaced.

## Test a configuration

Choose **Test connection** in the new or edit dialog to test the current draft without saving it. The check validates the local listen address and port, SSH authentication and jump hosts, and a TCP connection to the target service. It times out after 30 seconds and closes its temporary resources. Testing a running tunnel retains its existing local listener and uses a separate SSH transport, so existing forwarded sessions are not interrupted. TCP reachability does not validate database credentials or application health. Editing configuration fields clears the previous test result.

## Lifecycle and status

- Tunnels do not require a terminal tab or remote shell. Closing a terminal or the management dialog leaves tunnels running. Each plugin page shows live tunnel status.
- A saved tunnel has at most one running instance. Different tunnels have independent SSH transports.
- Quitting aTerm stops all tunnels and releases their ports. Closing the last application window normally quits aTerm under its existing window policy. If the application remains running, so do its tunnels.
- Configurations survive application restarts; tunnels do not automatically start or reconnect.
- Editing a running tunnel preserves the active endpoint. The list displays the active configuration and a **Restart** action when forwarding settings differ from the saved configuration. Restart closes existing forwarded connections.
- Starting, running, stopping, stopped, and failed states are shown separately. **Running** means the local listener and SSH transport are established; it does not certify the target application's health or authentication.
- Target connections are opened on demand with a 15 second timeout. A target connection error leaves the tunnel listening and displays the most recent error. A successful target connection clears it.
- SSH transport loss or keepalive failure stops forwarding and releases the listener. **Retry** starts a new connection. Tunnels use the host's keepalive settings, or a 15 second interval and three unanswered checks when the host disables keepalive.
- Delete a host’s associated tunnels before deleting that host. Deleting a tunnel stops its active connections.
- Encrypted backups include tunnel configurations. Restoring a backup stops current tunnels and restores configurations in the stopped state.

## Troubleshooting

| Message or symptom | Action |
| --- | --- |
| Local address already in use | Stop the application or tunnel holding that address, or edit the local port. |
| Local address unavailable or permission denied | Use an address assigned to this computer and a port your account can bind. |
| SSH authentication or host verification fails | Check the saved host, jump host, credentials, and server fingerprint. |
| Target connection refused or times out | Check the target address from the SSH host and verify the service is listening. |
| SSH server rejects forwarding | Ask the server administrator whether TCP forwarding to that target is permitted. |
| Running tunnel but service login fails | Check the external application's service credentials and protocol settings. |

Only local TCP forwarding is included. Reverse forwarding, SOCKS, automatic startup, automatic reconnect, and external tool launch are outside this version.

## Verification

Backend forwarding tests use disposable loopback SSH and TCP servers, including half-close handling, port conflicts, forwarding rejection, SSH disconnects, and cancellation. Database and service tests cover persistence, host reference protection, atomic snapshot validation, encrypted backup restoration, and runtime cleanup.

For native integration and UI checks, create a disposable profile:

```sh
aterm_fixture_profile="$(mktemp -d -t aterm-tunnels)"
python3 scripts/go-macos.py run ./scripts/parity-fixture -tunnels -profile "$aterm_fixture_profile"
```

Keep the fixture running. It prints the local HTTP endpoint and creates two synthetic SSH hosts, a jump-host tunnel, and an encrypted test vault. Launch a test build with `ATERM_DATA_DIR` set to that profile. The disposable vault password is `fixture-vault-password`. The fixture never uses the real host library or system Keychain.

For automated forwarding and lifecycle checks, run:

```sh
go test ./internal/desktop/plugins -run TestNative -v
go test ./plugins/ssh-tunnels/backend/forward -v
```

These launch real child processes and loopback targets. Native UI and signed
sandboxed distributions require separate platform acceptance.
