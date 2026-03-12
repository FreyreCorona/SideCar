package power

type PowerEvent int

const (
	Sleep PowerEvent = iota
	Wake
)

type Monitor interface {
	Start(callback func(PowerEvent)) error
	Stop()
}

func NewMonitor() Monitor {
	return newPowerMonitor()
}
