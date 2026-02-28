package blf

// GraphAvailability and GraphActivity are the values for Microsoft Graph setPresence.
const (
	GraphAvailabilityAvailable = "Available"
	GraphAvailabilityBusy      = "Busy"
	GraphActivityAvailable     = "Available"
	GraphActivityInACall       = "InACall"
)

// ToGraph maps BLF state to Graph availability and activity.
func (s State) ToGraph() (availability, activity string) {
	switch s {
	case StateIdle:
		return GraphAvailabilityAvailable, GraphActivityAvailable
	case StateRinging, StateBusy:
		return GraphAvailabilityBusy, GraphActivityInACall
	default:
		return GraphAvailabilityAvailable, GraphActivityAvailable
	}
}
