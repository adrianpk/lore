# Install and Activate Lore

Install the command and activate the per-user socket and verification timer.
This chapter uses a local source checkout because Lore does not yet define a
release distribution channel.

## Check the prerequisites

Lore requires Go, Git, and a Linux user session with systemd. Check them from
the repository root:

```sh
go version
git --version
systemctl --user --version
```

## Install the command

Install `lore` in the user-local binary directory used by the supplied systemd
units:

```sh
mkdir -p "$HOME/.local/bin"
GOBIN="$HOME/.local/bin" go install .
```

Confirm that the shell can find the command:

```sh
command -v lore
```

The result should identify `$HOME/.local/bin/lore`. Add `$HOME/.local/bin` to
`PATH` when the command is not found.

## Install the user units

Copy the supplied units from the checkout:

```sh
install -Dm0644 assets/systemd/user/lore.service \
  "$HOME/.config/systemd/user/lore.service"
install -Dm0644 assets/systemd/user/lore.socket \
  "$HOME/.config/systemd/user/lore.socket"
install -Dm0644 assets/systemd/user/lore-verify.service \
  "$HOME/.config/systemd/user/lore-verify.service"
install -Dm0644 assets/systemd/user/lore-verify.timer \
  "$HOME/.config/systemd/user/lore-verify.timer"
```

Reload the user manager and activate socket-based startup and scheduled
verification:

```sh
systemctl --user daemon-reload
systemctl --user enable --now lore.socket lore-verify.timer
```

## Check the activation

```sh
systemctl --user is-active lore.socket lore-verify.timer
```

Both units should report `active`. The service starts on the first client
request. The timer runs verification at 03:00 in the machine's local timezone.

Continue with [Connect a workspace agent](02-connect-a-workspace-agent.md).
