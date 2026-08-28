package blf

// GraphAvailability and GraphActivity are the values for Microsoft Graph setPresence.
const (
	GraphAvailabilityAvailable = "Available"
	GraphAvailabilityBusy      = "Busy"
	GraphActivityAvailable     = "Available"
	GraphActivityInACall       = "InACall"
)

// ToGraph maps BLF state to Graph availability and activity.
// Ringing stays Available so Teams only shows Busy when the line is answered.
func (s State) ToGraph() (availability, activity string) {
	switch s {
	case StateBusy:
		return GraphAvailabilityBusy, GraphActivityInACall
	case StateIdle, StateRinging:
		return GraphAvailabilityAvailable, GraphActivityAvailable
	default:
		return GraphAvailabilityAvailable, GraphActivityAvailable
	}
}
