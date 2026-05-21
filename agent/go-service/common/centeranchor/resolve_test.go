package centeranchor

import "testing"

func TestResolveRect(t *testing.T) {
	const imgW, imgH = 1280, 720
	cases := []struct {
		name   string
		roi    [4]int
		center [2]bool
		want   [4]int
	}{
		{"absolute", [4]int{100, 200, 50, 60}, [2]bool{false, false}, [4]int{100, 200, 50, 60}},
		{"negative passthrough", [4]int{-100, -50, 80, 40}, [2]bool{false, false}, [4]int{-100, -50, 80, 40}},
		{"center x only", [4]int{-225, 100, 450, 100}, [2]bool{true, false}, [4]int{415, 100, 450, 100}},
		{"center y only", [4]int{200, -160, 250, 400}, [2]bool{false, true}, [4]int{200, 200, 250, 400}},
		{"center both", [4]int{-225, 260, 450, 100}, [2]bool{true, true}, [4]int{415, 620, 450, 100}},
		{"center+negative mix", [4]int{-250, -160, 250, 400}, [2]bool{false, true}, [4]int{-250, 200, 250, 400}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRect(tc.roi, tc.center, imgW, imgH)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.X() != tc.want[0] || got.Y() != tc.want[1] || got.Width() != tc.want[2] || got.Height() != tc.want[3] {
				t.Errorf("ResolveRect(%v, %v) = [%d %d %d %d], want %v",
					tc.roi, tc.center, got.X(), got.Y(), got.Width(), got.Height(), tc.want)
			}
		})
	}
}

func TestResolveRectInvalidSize(t *testing.T) {
	if _, err := ResolveRect([4]int{0, 0, 1, 1}, [2]bool{false, false}, 0, 720); err == nil {
		t.Error("expected error for imgW=0")
	}
	if _, err := ResolveRect([4]int{0, 0, 1, 1}, [2]bool{false, false}, 1280, 0); err == nil {
		t.Error("expected error for imgH=0")
	}
}

func TestResolvePoint(t *testing.T) {
	const imgW, imgH = 1280, 720
	x, y, err := ResolvePoint([2]int{-225, 260}, [2]bool{true, true}, imgW, imgH)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if x != 415 || y != 620 {
		t.Errorf("ResolvePoint = (%d, %d), want (415, 620)", x, y)
	}
}
