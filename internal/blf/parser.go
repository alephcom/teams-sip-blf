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
// The document uses namespace urn:ietf:params:xml:ns:dialog-info; dialog
// state is a child element <state>, not an attribute.
type DialogInfo struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:dialog-info dialog-info"`
	Entity  string   `xml:"entity,attr"` // e.g. sip:1001@server
	Dialogs []Dialog `xml:"urn:ietf:params:xml:ns:dialog-info dialog"`
}

// Dialog represents a single dialog in the dialog-info document.
// Per RFC 4235, the dialog state is a child <state> element (e.g. <state>confirmed</state>).
// StateAttr supports PBXs that send state as an attribute on <dialog>.
type Dialog struct {
	ID        string `xml:"id,attr"`
	State     string `xml:"urn:ietf:params:xml:ns:dialog-info state"` // child element content
	StateAttr string `xml:"state,attr"` // optional; some PBXs send state as attribute
	Direction string `xml:"direction,attr"`
	Local     struct {
		Identity string `xml:"urn:ietf:params:xml:ns:dialog-info identity"`
		Target   string `xml:"urn:ietf:params:xml:ns:dialog-info target"`
	} `xml:"urn:ietf:params:xml:ns:dialog-info local"`
	Remote struct {
		Identity string `xml:"urn:ietf:params:xml:ns:dialog-info identity"`
		Target   string `xml:"urn:ietf:params:xml:ns:dialog-info target"`
	} `xml:"urn:ietf:params:xml:ns:dialog-info remote"`
}

// dialogState returns the effective dialog state (child <state> element or state attribute).
func (d *Dialog) dialogState() string {
	s := strings.TrimSpace(d.State)
	if s == "" {
		s = strings.TrimSpace(d.StateAttr)
	}
	return strings.ToLower(s)
}

// dialogNoNS is used when the document has no default namespace (some PBXs omit xmlns).
type dialogNoNS struct {
	ID        string `xml:"id,attr"`
	State     string `xml:"state"`
	StateAttr string `xml:"state,attr"`
}

type dialogInfoNoNS struct {
	XMLName xml.Name   `xml:"dialog-info"`
	Entity  string     `xml:"entity,attr"`
	Dialogs []dialogNoNS `xml:"dialog"`
}

// ParseDialogInfo parses RFC 4235 dialog-info XML and returns the effective
// BLF state: idle (no dialogs or all terminated), ringing (early/trying), or busy (confirmed).
// Uses the RFC namespace first; if unmarshal fails (e.g. PBX omits xmlns), retries without namespace.
func ParseDialogInfo(body []byte) State {
	var info DialogInfo
	if err := xml.Unmarshal(body, &info); err == nil {
		return dialogsToState(info.Dialogs)
	}
	var infoNoNS dialogInfoNoNS
	if err := xml.Unmarshal(body, &infoNoNS); err != nil {
		return StateUnknown
	}
	return dialogsNoNSToState(infoNoNS.Dialogs)
}

func dialogsToState(dialogs []Dialog) State {
	if len(dialogs) == 0 {
		return StateIdle
	}
	for _, d := range dialogs {
		s := d.dialogState()
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

func dialogStateStr(s, sAttr string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		s = strings.TrimSpace(strings.ToLower(sAttr))
	}
	return s
}

func dialogsNoNSToState(dialogs []dialogNoNS) State {
	if len(dialogs) == 0 {
		return StateIdle
	}
	for _, d := range dialogs {
		s := dialogStateStr(d.State, d.StateAttr)
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
