package board

import "testing"

// The shared native Makefile builds for these APIs only.
func TestEveryBoardNamesABuildableNativeAPI(t *testing.T) {
	for _, b := range boards {
		switch b.NativeAPI {
		case 25, 27, 28:
		default:
			t.Errorf("%s wants native API %d, which the helpers do not build for",
				b.Name, b.NativeAPI)
		}
	}
}

func TestSecureVideo(t *testing.T) {
	tests := []struct {
		board Board
		want  bool
	}{
		{Blueberry, true},
		{Opal, true},
		{Ivy, true},
		{Penghu, false},
		{Helios, false},
		{Board{}, false},
	}
	for _, tt := range tests {
		if got := tt.board.Has(SecureVideo); got != tt.want {
			t.Errorf("%s.Has(SecureVideo) = %v, want %v", tt.board.Name, got, tt.want)
		}
	}
	if !Penghu.Has(0) {
		t.Errorf("Penghu.Has(0) = false, want true")
	}
}

func TestOnlyTheClockHasADock(t *testing.T) {
	for _, b := range boards {
		if got, want := b.Has(Dock), b.Name == "helios"; got != want {
			t.Errorf("%s.Has(Dock) = %v, want %v", b.Name, got, want)
		}
	}
}
