package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
)

type helloField string

const (
	helloFieldDeviceOS   helloField = "device_os"
	helloFieldName       helloField = "name"
	helloFieldPackage    helloField = "package_name"
	helloFieldAndroidID  helloField = "android_id"
	helloFieldHost       helloField = "host"
	helloFieldRemotePort helloField = "remote_port"
)

type helloLayout struct {
	Name string
	Tags map[helloField]int
}

type helloFieldSet struct {
	Name   string
	Fields []helloField
}

type helloValues struct {
	DeviceOS    int
	Name        string
	PackageName string
	AndroidID   string
	Host        string
	RemotePort  int
}

type helloPayloadCandidate struct {
	Bytes       []byte
	Description string
}

var knownHelloLayouts = map[string]helloLayout{
	"os-name-package-android-host-port": {
		Name: "os-name-package-android-host-port",
		Tags: map[helloField]int{
			helloFieldDeviceOS:   1,
			helloFieldName:       2,
			helloFieldPackage:    3,
			helloFieldAndroidID:  4,
			helloFieldHost:       5,
			helloFieldRemotePort: 6,
		},
	},
	"name-os-package-android-host-port": {
		Name: "name-os-package-android-host-port",
		Tags: map[helloField]int{
			helloFieldName:       1,
			helloFieldDeviceOS:   2,
			helloFieldPackage:    3,
			helloFieldAndroidID:  4,
			helloFieldHost:       5,
			helloFieldRemotePort: 6,
		},
	},
	"name-package-android-os-host-port": {
		Name: "name-package-android-os-host-port",
		Tags: map[helloField]int{
			helloFieldName:       1,
			helloFieldPackage:    2,
			helloFieldAndroidID:  3,
			helloFieldDeviceOS:   4,
			helloFieldHost:       5,
			helloFieldRemotePort: 6,
		},
	},
}

func buildHelloAttempts(cfg config) []attempt {
	payloads := buildHelloPayloadCandidates(cfg)
	attempts := make([]attempt, 0, len(payloads)*len(cfg.frameStyles)*len(cfg.serviceTags)*len(cfg.commandTags)*len(cfg.payloadTags)*len(cfg.serviceIDs)*len(cfg.commandIDs))
	for _, frameStyle := range cfg.frameStyles {
		for _, serviceTag := range cfg.serviceTags {
			for _, commandTag := range cfg.commandTags {
				if commandTag == serviceTag {
					continue
				}
				for _, serviceID := range cfg.serviceIDs {
					for _, commandID := range cfg.commandIDs {
						for _, payloadTag := range cfg.payloadTags {
							if payloadTag <= 0 || payloadTag == serviceTag || payloadTag == commandTag {
								continue
							}
							for _, payload := range payloads {
								attempts = append(attempts, attempt{
									FrameStyle:       frameStyle,
									ServiceTag:       serviceTag,
									CommandTag:       commandTag,
									PayloadTag:       payloadTag,
									ServiceID:        serviceID,
									CommandID:        commandID,
									Template:         "hello",
									PayloadDesc:      payload.Description,
									HasCustomPayload: true,
									PayloadBytes:     payload.Bytes,
								})
							}
						}
					}
				}
			}
		}
	}
	return attempts
}

func attachHelloPreambles(cfg config, attempts []attempt) []attempt {
	payloads := buildHelloPayloadCandidates(cfg)
	if len(payloads) == 0 || len(attempts) == 0 {
		return attempts
	}
	withPreambles := make([]attempt, 0, len(payloads)*len(attempts))
	for _, attempt := range attempts {
		for _, payload := range payloads {
			candidate := attempt
			candidate.Template = fmt.Sprintf("%s+hello-preamble", cfg.mode)
			candidate.PreambleDesc = payload.Description
			candidate.PreambleBytes = buildCanonicalHelloRaw(payload.Bytes)
			withPreambles = append(withPreambles, candidate)
		}
	}
	return withPreambles
}

func buildCanonicalHelloRaw(payload []byte) []byte {
	msg := make([]byte, 0, len(payload)+8)
	msg = append(msg, fieldVarint(1, 1)...)
	msg = append(msg, fieldVarint(2, 0)...)
	msg = append(msg, fieldBytes(3, payload)...)
	return msg
}

func buildHelloPayloadCandidates(cfg config) []helloPayloadCandidate {
	layouts, err := resolveHelloLayouts(cfg.helloLayouts)
	if err != nil {
		return nil
	}
	localHost := strings.TrimSpace(cfg.helloHostValue)
	if localHost == "" {
		localHost = detectLocalIP(cfg.host, cfg.port)
	}

	candidates := make([]helloPayloadCandidate, 0)
	seen := map[string]struct{}{}
	addCandidate := func(payload []byte, description string) {
		key := hex.EncodeToString(payload)
		if _, ok := seen[key]; ok {
			return
		}
		copyPayload := append([]byte(nil), payload...)
		candidates = append(candidates, helloPayloadCandidate{Bytes: copyPayload, Description: description})
		seen[key] = struct{}{}
	}

	for _, capabilityTag := range cfg.helloCapabilityTags {
		for _, capabilityValue := range cfg.helloCapabilityValues {
			addCandidate(
				fieldVarint(capabilityTag, capabilityValue),
				fmt.Sprintf("capability-only(capability_tag=%d,capability=%d)", capabilityTag, capabilityValue),
			)
		}
	}

	for _, layout := range layouts {
		for _, devInfoTag := range cfg.helloDevInfoTags {
			addCandidate(
				fieldBytes(devInfoTag, nil),
				fmt.Sprintf("devinfo-empty(devinfo_tag=%d,layout=%s)", devInfoTag, layout.Name),
			)

			for _, fieldSet := range helloFieldSets() {
				deviceOSValues := []int{0}
				if helloFieldSetUses(fieldSet, helloFieldDeviceOS) {
					deviceOSValues = cfg.helloDeviceOSValues
				}
				remotePorts := []int{0}
				if helloFieldSetUses(fieldSet, helloFieldRemotePort) {
					remotePorts = cfg.helloRemotePorts
				}

				for _, deviceOSValue := range deviceOSValues {
					for _, remotePort := range remotePorts {
						values := helloValues{
							DeviceOS:    deviceOSValue,
							Name:        cfg.helloName,
							PackageName: cfg.helloPackageName,
							AndroidID:   cfg.helloAndroidID,
							Host:        localHost,
							RemotePort:  remotePort,
						}
						devInfoBytes, ok := buildHelloDevInfo(layout, fieldSet, values)
						if !ok {
							continue
						}

						basePayload := fieldBytes(devInfoTag, devInfoBytes)
						baseDescription := fmt.Sprintf(
							"devinfo(devinfo_tag=%d,layout=%s,fields=%s,device_os=%d,remote_port=%d)",
							devInfoTag,
							layout.Name,
							fieldSet.Name,
							deviceOSValue,
							remotePort,
						)
						addCandidate(basePayload, baseDescription)

						for _, capabilityTag := range cfg.helloCapabilityTags {
							if capabilityTag == devInfoTag {
								continue
							}
							for _, capabilityValue := range cfg.helloCapabilityValues {
								payload := append(append([]byte(nil), basePayload...), fieldVarint(capabilityTag, capabilityValue)...)
								description := fmt.Sprintf(
									"devinfo+capability(devinfo_tag=%d,capability_tag=%d,layout=%s,fields=%s,device_os=%d,capability=%d,remote_port=%d)",
									devInfoTag,
									capabilityTag,
									layout.Name,
									fieldSet.Name,
									deviceOSValue,
									capabilityValue,
									remotePort,
								)
								addCandidate(payload, description)
							}
						}
					}
				}
			}
		}
	}

	return candidates
}

func resolveHelloLayouts(names []string) ([]helloLayout, error) {
	layouts := make([]helloLayout, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		layout, ok := knownHelloLayouts[name]
		if !ok {
			return nil, fmt.Errorf("unknown hello layout %q", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		layouts = append(layouts, layout)
		seen[name] = struct{}{}
	}
	if len(layouts) == 0 {
		return nil, fmt.Errorf("no hello layouts selected")
	}
	return layouts, nil
}

func helloFieldSets() []helloFieldSet {
	return []helloFieldSet{
		{Name: "os", Fields: []helloField{helloFieldDeviceOS}},
		{Name: "name", Fields: []helloField{helloFieldName}},
		{Name: "os-name", Fields: []helloField{helloFieldDeviceOS, helloFieldName}},
		{Name: "name-package", Fields: []helloField{helloFieldName, helloFieldPackage}},
		{Name: "os-name-package", Fields: []helloField{helloFieldDeviceOS, helloFieldName, helloFieldPackage}},
		{Name: "os-name-package-android", Fields: []helloField{helloFieldDeviceOS, helloFieldName, helloFieldPackage, helloFieldAndroidID}},
		{Name: "os-name-package-android-host", Fields: []helloField{helloFieldDeviceOS, helloFieldName, helloFieldPackage, helloFieldAndroidID, helloFieldHost}},
		{Name: "os-name-package-android-host-port", Fields: []helloField{helloFieldDeviceOS, helloFieldName, helloFieldPackage, helloFieldAndroidID, helloFieldHost, helloFieldRemotePort}},
	}
}

func helloFieldSetUses(fieldSet helloFieldSet, field helloField) bool {
	return slices.Contains(fieldSet.Fields, field)
}

func buildHelloDevInfo(layout helloLayout, fieldSet helloFieldSet, values helloValues) ([]byte, bool) {
	type encodedField struct {
		Tag   int
		Bytes []byte
	}

	encoded := make([]encodedField, 0, len(fieldSet.Fields))
	for _, field := range fieldSet.Fields {
		tag, ok := layout.Tags[field]
		if !ok {
			return nil, false
		}
		switch field {
		case helloFieldDeviceOS:
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldVarint(tag, values.DeviceOS)})
		case helloFieldName:
			if values.Name == "" {
				return nil, false
			}
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldString(tag, values.Name)})
		case helloFieldPackage:
			if values.PackageName == "" {
				return nil, false
			}
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldString(tag, values.PackageName)})
		case helloFieldAndroidID:
			if values.AndroidID == "" {
				return nil, false
			}
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldString(tag, values.AndroidID)})
		case helloFieldHost:
			if values.Host == "" {
				return nil, false
			}
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldString(tag, values.Host)})
		case helloFieldRemotePort:
			encoded = append(encoded, encodedField{Tag: tag, Bytes: fieldVarint(tag, values.RemotePort)})
		}
	}

	sort.Slice(encoded, func(i, j int) bool {
		return encoded[i].Tag < encoded[j].Tag
	})

	payload := make([]byte, 0, len(encoded)*8)
	for _, field := range encoded {
		payload = append(payload, field.Bytes...)
	}
	return payload, true
}

func fieldString(tag int, value string) []byte {
	return fieldBytes(tag, []byte(value))
}

func detectLocalIP(targetHost string, targetPort int) string {
	if strings.TrimSpace(targetHost) == "" {
		return ""
	}
	if targetPort <= 0 {
		targetPort = 8987
	}
	conn, err := net.Dial("udp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)))
	if err != nil {
		return ""
	}
	defer conn.Close()
	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || udpAddr.IP == nil {
		return ""
	}
	return udpAddr.IP.String()
}
