package manifest

import "testing"

func TestCommandSpecIsEmpty(t *testing.T) {
	if !(CommandSpec{}).IsEmpty() {
		t.Fatal("expected zero command spec to be empty")
	}

	if (CommandSpec{Command: "echo hi"}).IsEmpty() {
		t.Fatal("expected non-empty command to be non-empty")
	}

	if (CommandSpec{CommandByOS: map[string]string{"linux": "   ", "default": "\t"}}).IsEmpty() != true {
		t.Fatal("expected whitespace-only command_by_os to be empty")
	}

	if (CommandSpec{CommandByOS: map[string]string{"linux": "bun run start"}}).IsEmpty() {
		t.Fatal("expected command_by_os with value to be non-empty")
	}
}
