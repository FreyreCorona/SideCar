//go:build linux

package power

import (
	"bufio"
	"log"
	"os/exec"
	"strings"
)

type linuxPowerMonitor struct {
	cmd *exec.Cmd
}

func newPowerMonitor() Monitor {
	return &linuxPowerMonitor{}
}

func (m *linuxPowerMonitor) Start(callback func(PowerEvent)) error {
	m.cmd = exec.Command("dbus-monitor", "--system", "type='signal',interface='org.freedesktop.login1.Manager',member='PrepareForSleep'")

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := m.cmd.Start(); err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "boolean true") {
				log.Println("PowerMonitor: system is going to sleep")
				callback(Sleep)
			} else if strings.Contains(line, "boolean false") {
				log.Println("PowerMonitor: system is waking up")
				callback(Wake)
			}
		}
	}()

	return nil
}

func (m *linuxPowerMonitor) Stop() {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}
