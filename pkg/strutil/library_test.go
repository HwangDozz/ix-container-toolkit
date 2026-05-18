package strutil

import "testing"

func TestIsSharedLibrary(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"libcuda.so", true},
		{"libcuda.so.1", true},
		{"libcuda.so.1.2.3", true},
		{"libthunk.so", true},
		{"readme.txt", false},
		{"libcuda.a", false},
		{"libcuda.so.bak", false},
		{"libcuda.something", false},
		{"", false},
		{".so", true},
		{".so.1", true},
		{"libfoo.so.", false},
	}
	for _, tt := range tests {
		got := IsSharedLibrary(tt.name)
		if got != tt.want {
			t.Errorf("IsSharedLibrary(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
