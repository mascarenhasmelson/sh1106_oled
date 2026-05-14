```
sudo tee /etc/systemd/system/oled-monitor.service <<EOF
[Unit]
Description=OLED System Monitor
After=network.target

[Service]
ExecStart=/home/devtest/sh1106/monitor
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now oled-monitor
```
