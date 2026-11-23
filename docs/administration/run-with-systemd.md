# Run with Systemd

For non-Docker users, you could run QingyuGist as a systemd service.

QingyuGist takes everything from its environment and from the admin panel; there
is no configuration file to install. Only the home directory has to be set at
boot time, with `QYG_HOME`.

## As root
On Unix distributions with systemd, place the QingyuGist binary like:

```shell
sudo cp qingyugist /usr/local/bin
```

Create a new user to run QingyuGist and its home directory:

```shell
sudo useradd --system qingyugist
sudo mkdir -p /var/lib/qingyugist
sudo chown -R qingyugist:qingyugist /var/lib/qingyugist
```

Then create a service file at `/etc/systemd/system/qingyugist.service`:

```ini
[Unit]
Description=qingyugist Server
After=network.target

[Service]
Type=simple
User=qingyugist
Group=qingyugist
Environment=QYG_HOME=/var/lib/qingyugist
ExecStart=/usr/local/bin/qingyugist
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Add any other boot-time options as `Environment=` lines (see
[Configuration](/configuration/configure)); everything else is changed in the
admin panel at `/admin/settings`.

Finally, start the service:

```shell
systemctl daemon-reload
systemctl enable --now qingyugist
systemctl status qingyugist
```

----

## As a normal user
**NOTE: This was tested on Ubuntu 20.04 and newer. For other distros, please check the respective documentation**  

#### For the purpose of this documentation, we will assume that:
- You've followed the instructions on how to run qingyugist [from source](https://github.com/qingyu-org/Gist?tab=readme-ov-file#from-source)
- Your shell user is named `pastebin`
- All commands are being executed as the `pastebin` user

_If none of the above is true, then adapt the commands and paths to fit your needs._

Enable lingering for the user:
```shell
loginctl enable-linger
```

Create the user systemd folder:
```
mkdir -p /home/pastebin/.config/systemd/user
```

Then create a service file at `/home/pastebin/.config/systemd/user/qingyugist.service`:
```ini
[Unit]
Description=qingyugist Server
After=network.target

[Service]
Type=simple
ExecStart=/home/pastebin/qingyugist/qingyugist
Restart=on-failure

[Install]
WantedBy=default.target
```

Finally, start the service:
```shell
systemctl --user daemon-reload
systemctl --user enable --now qingyugist
systemctl --user status qingyugist
```
