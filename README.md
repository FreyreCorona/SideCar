# SideCar

SideCar is a modern, lightweight, and extensible utility designed to unlock the full potential of auxiliary displays, such as the secondary screens found on devices like the **Positivo Vision R15 M**.

Unlike generic monitor tools, SideCar is a specialized rendering engine that transforms raw system metrics into beautiful, structured dashboards.

## Key Features

- **Modern Dashboard UI:** A refined interface with indigo/violet accents, optimized for high legibility and professional aesthetics.
- **Native Metrics:** "Pure Go" implementation for collecting CPU, RAM, Battery, and Network metrics without external binary dependencies.
- **Advanced WiFi Monitoring:** Native SSID detection and signal quality monitoring (Linux).
- **Power Intelligence:** Automatically manages device energy by detecting system sleep and wake events (Cross-platform).
- **Responsive Design:** Adaptive interface that works perfectly in full screen or compact window modes.
- **Lightweight & Portable:** Written in Go and JavaScript, designed to be fast and low-impact on system resources.

## Setup & Permissions

To communicate with the auxiliary hardware via the serial port, SideCar requires specific system permissions.

### 🪟 Windows
On Windows, the serial communication is restricted to administrative processes.
- **Requirement:** Right-click the `SideCar.exe` and select **"Run as Administrator"**.

### 🐧 Linux
Linux systems restrict access to `/dev/ttyACM*` devices by default. You have two ways to grant access:

#### Option A: Add your user to the `dialout` group (Recommended)
This is the standard way to grant permanent access to serial ports.
1. Run the following command in your terminal:
   ```bash
   sudo usermod -a -G dialout $USER
   ```
2. **Important:** You must log out and log back in for the changes to take effect.

#### Option B: Temporary access (Until reboot)
If you just want to test it quickly:
```bash
sudo chmod 666 /dev/ttyACM0
```
*(Replace `ttyACM0` with your actual device port if different)*

---

## Philosophy

An auxiliary screen should not just be a decorative element; it is a vital information surface. SideCar exists to provide a bridge between your system's heartbeat and that surface, focusing on **security**, **performance**, and **clean design**.

## Project Status

🚧 **Active Development.** Features and dashboard layouts are being constantly refined to match professional standards.

---
© 2026 SideCar Project. Built for performance and style.
