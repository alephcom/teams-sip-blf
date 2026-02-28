package blf

import (
	"testing"
)

func TestParseDialogInfo_StateElement(t *testing.T) {
	// RFC 4235: state is a child <state> element, document uses default namespace
	confirmed := []byte(`<?xml version="1.0"?>
<dialog-info xmlns="urn:ietf:params:xml:ns:dialog-info" version="1" state="full" entity="sip:6000@pbx.example.com">
  <dialog id="abc123" direction="recipient">
    <state>confirmed</state>
  </dialog>
</dialog-info>`)
	if got := ParseDialogInfo(confirmed); got != StateBusy {
		t.Errorf("ParseDialogInfo(confirmed) = %v, want Busy", got)
	}

	terminated := []byte(`<?xml version="1.0"?>
<dialog-info xmlns="urn:ietf:params:xml:ns:dialog-info" version="2" state="full" entity="sip:6000@pbx.example.com">
  <dialog id="abc123" direction="recipient">
    <state>terminated</state>
  </dialog>
</dialog-info>`)
	if got := ParseDialogInfo(terminated); got != StateIdle {
		t.Errorf("ParseDialogInfo(terminated) = %v, want Idle", got)
	}

	early := []byte(`<?xml version="1.0"?>
<dialog-info xmlns="urn:ietf:params:xml:ns:dialog-info" version="0" state="full" entity="sip:6000@pbx.example.com">
  <dialog id="xyz" direction="initiator">
    <state>early</state>
  </dialog>
</dialog-info>`)
	if got := ParseDialogInfo(early); got != StateRinging {
		t.Errorf("ParseDialogInfo(early) = %v, want Ringing", got)
	}
}

func TestParseDialogInfo_NoNamespace(t *testing.T) {
	// Some PBXs omit xmlns; state as child element
	body := []byte(`<?xml version="1.0"?>
<dialog-info version="1" state="full" entity="sip:6000@pbx">
  <dialog id="x">
    <state>confirmed</state>
  </dialog>
</dialog-info>`)
	if got := ParseDialogInfo(body); got != StateBusy {
		t.Errorf("ParseDialogInfo(no namespace) = %v, want Busy", got)
	}
}

func TestExtensionFromDialogInfo(t *testing.T) {
	body := []byte(`<?xml version="1.0"?>
<dialog-info xmlns="urn:ietf:params:xml:ns:dialog-info" version="1" state="full" entity="sip:6000@pbx.example.com">
  <dialog id="abc"><state>confirmed</state></dialog>
</dialog-info>`)
	if got := ExtensionFromDialogInfo(body); got != "6000" {
		t.Errorf("ExtensionFromDialogInfo = %q, want 6000", got)
	}
}
