package platform

import "testing"

func TestInstalledStateRootForLinux(t *testing.T) {
	got := installedStateRootFor("linux", "/home/jose", "", "")
	want := "/home/jose/.local/share/needlex"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInstalledStateRootForMac(t *testing.T) {
	got := installedStateRootFor("darwin", "/Users/jose", "", "")
	want := "/Users/jose/Library/Application Support/NeedleX"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInstalledStateRootForWindows(t *testing.T) {
	got := installedStateRootFor("windows", `C:\Users\jose`, "", `C:\Users\jose\AppData\Local`)
	want := `C:\Users\jose\AppData\Local\NeedleX`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
