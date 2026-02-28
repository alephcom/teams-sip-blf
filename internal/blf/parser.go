package blf

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// State is the normalized BLF state for an extension.
type State string

const (
	StateIdle    State = "idle"
	StateRinging State = "ringing"
	StateBusy    State = "busy"
	StateUnknown State = "unknown"
)

// Event represents a BLF state change (e.g. from a NOTIFY body).
type Event struct {
	Extension string
	State     State
}

// DialogInfo is the RFC 4235 dialog event package XML (simplified).
// Full spec: https://www.rfc-editor.org/rfc/rfc4235
type DialogInfo struct {
	XMLName xml.Name `xml:"dialog-info"`
	Entity  string  `xml:"entity,attr"` // e.g. sip:1001@server
	Dialogs []Dialog `xml:"dialog"`
}

// Dialog represents a single dialog in the dialog-info document.
type Dialog struct {
	ID     string `xml:"id,attr"`
	State  string `xml:"state,attr"`  // full, partial, terminated
	Direction string `xml:"direction,attr"`
	Local  struct {
		Identity string `xml:"identity"`
		Target   string `xml:"target"`
	} `xml:"local"`
	Remote struct {
		Identity string `xml:"identity"`
		Target   string `xml:"target"`
	} `xml:"remote"`
}

// ParseDialogInfo parses RFC 4235 dialog-info XML and returns the effective
// BLF state: idle (no dialogs or all terminated), ringing (early/trying), or busy (confirmed).
func ParseDialogInfo(body []byte) State {
	var info DialogInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return StateUnknown
	}
	if len(info.Dialogs) == 0 {
		return StateIdle
	}
	for _, d := range info.Dialogs {
		s := strings.ToLower(d.State)
		switch {
		case s == "terminated" || s == "":
			continue
		case s == "trying" || s == "early" || s == "proceeding":
			return StateRinging
		case s == "confirmed":
			return StateBusy
		default:
			return StateBusy
		}
	}
	return StateIdle
}

// ExtensionFromDialogInfo parses dialog-info XML and returns the entity/extension
// (e.g. "1001") from the entity attribute or the first dialog's local identity.
func ExtensionFromDialogInfo(body []byte) string {
	var info DialogInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return ""
	}
	if info.Entity != "" {
		// entity is e.g. "sip:1001@pbx.example.com"
		if idx := strings.Index(info.Entity, ":"); idx >= 0 {
			rest := info.Entity[idx+1:]
			if at := strings.Index(rest, "@"); at >= 0 {
				return rest[:at]
			}
			return rest
		}
	}
	if len(info.Dialogs) > 0 && info.Dialogs[0].Local.Identity != "" {
		ident := info.Dialogs[0].Local.Identity
		if idx := strings.Index(ident, ":"); idx >= 0 {
			rest := ident[idx+1:]
			if at := strings.Index(rest, "@"); at >= 0 {
				return rest[:at]
			}
			return rest
		}
	}
	return ""
}

// ParsePresenceBody parses a presence event body (RFC 3856 style) if needed.
// Some PBXs send presence instead of dialog. This is a minimal parser;
// extend if your PBX uses presence for BLF.
func ParsePresenceBody(body []byte) State {
	if bytes.Contains(body, []byte("dialog-info")) {
		return ParseDialogInfo(body)
	}
	if bytes.Contains(body, []byte("closed")) && !bytes.Contains(body, []byte("open")) {
		return StateIdle
	}
	if bytes.Contains(body, []byte("open")) {
		return StateBusy
	}
	return StateUnknown
}
