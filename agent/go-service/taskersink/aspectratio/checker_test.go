package aspectratio

import "testing"

func TestChooseScreenshotOption(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		want   string
	}{
		{"exact 1280x720", 1280, 720, "16:9"},
		{"exact 1920x1080", 1920, 1080, "16:9"},
		{"4k 16:9", 3840, 2160, "16:9"},
		{"21:9 wide", 2560, 1080, "wider_than_16x9"},
		{"32:9 ultrawide", 3840, 1080, "wider_than_16x9"},
		{"16:10", 1920, 1200, "narrower_than_16x9"},
		{"4:3", 1280, 960, "narrower_than_16x9"},
		{"5:4", 1280, 1024, "narrower_than_16x9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, mode := chooseScreenshotOption(tc.width, tc.height)
			if mode != tc.want {
				t.Errorf("chooseScreenshotOption(%d, %d) mode = %q, want %q",
					tc.width, tc.height, mode, tc.want)
			}
		})
	}
}
