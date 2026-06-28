package androidtv

import (
	"testing"

	"shrmt/core/action"
)

func TestPlanSendSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		act      action.Action
		powered  bool
		hasPower bool
		want     []sendStep
	}{
		{
			name:     "home on awake device stays home",
			act:      action.Home,
			powered:  true,
			hasPower: true,
			want:     []sendStep{{key: action.Home.String()}},
		},
		{
			name:     "home on sleeping device uses app launch wake pulse",
			act:      action.Home,
			powered:  false,
			hasPower: true,
			want: []sendStep{
				{appLink: wakeLaunchPackage, postDelay: wakeKeyDelay},
				{key: action.Home.String()},
			},
		},
		{
			name:     "power on sleeping device uses app launch wake pulse",
			act:      action.Power,
			powered:  false,
			hasPower: true,
			want: []sendStep{
				{appLink: wakeLaunchPackage, postDelay: wakeKeyDelay},
				{key: action.Home.String()},
			},
		},
		{
			name:     "power without state stays power",
			act:      action.Power,
			powered:  false,
			hasPower: false,
			want:     []sendStep{{key: action.Power.String()}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := planSendSteps(tt.act, tt.powered, tt.hasPower)
			if err != nil {
				t.Fatalf("planSendSteps(%q) returned error: %v", tt.act, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("planSendSteps(%q) returned %d steps, want %d", tt.act, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("planSendSteps(%q)[%d] = %+v, want %+v", tt.act, i, got[i], tt.want[i])
				}
			}
		})
	}
}
