package main

import (
	"encoding/hex"
	"testing"
)

func TestBuildHelloDevInfo(t *testing.T) {
	layouts, err := resolveHelloLayouts([]string{"os-name-package-android-host-port"})
	if err != nil {
		t.Fatalf("resolve hello layouts: %v", err)
	}
	fieldSet := helloFieldSet{
		Name:   "os-name-package-android",
		Fields: []helloField{helloFieldDeviceOS, helloFieldName, helloFieldPackage, helloFieldAndroidID},
	}
	got, ok := buildHelloDevInfo(layouts[0], fieldSet, helloValues{
		DeviceOS:    2,
		Name:        "shrmt",
		PackageName: "com.nvidia.shield.remote",
		AndroidID:   "aid",
		Host:        "192.168.1.18",
		RemotePort:  8987,
	})
	if !ok {
		t.Fatal("expected hello devInfo build to succeed")
	}
	want := "080212057368726d741a18636f6d2e6e76696469612e736869656c642e72656d6f74652203616964"
	if gotHex := hex.EncodeToString(got); gotHex != want {
		t.Fatalf("unexpected devInfo hex\nwant: %s\n got: %s", want, gotHex)
	}
}

func TestBuildHelloPayloadCandidatesIncludesExpectedPayloads(t *testing.T) {
	cfg := config{
		host:                  "192.168.1.16",
		port:                  8987,
		helloDevInfoTags:      []int{1},
		helloCapabilityTags:   []int{2},
		helloLayouts:          []string{"os-name-package-android-host-port"},
		helloName:             "shrmt",
		helloPackageName:      "com.nvidia.shield.remote",
		helloAndroidID:        "aid",
		helloHostValue:        "192.168.1.18",
		helloRemotePorts:      []int{8987},
		helloDeviceOSValues:   []int{2},
		helloCapabilityValues: []int{1},
	}

	candidates := buildHelloPayloadCandidates(cfg)
	got := map[string]struct{}{}
	for _, candidate := range candidates {
		got[hex.EncodeToString(candidate.Bytes)] = struct{}{}
	}

	wants := []string{
		"1001",
		"0a00",
		"0a020802",
		"0a09080212057368726d741001",
		"0a28080212057368726d741a18636f6d2e6e76696469612e736869656c642e72656d6f746522036169641001",
	}
	for _, want := range wants {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected payload %s in candidate set", want)
		}
	}
}
