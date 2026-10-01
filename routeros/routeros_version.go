package routeros

import (
	"fmt"
	"strconv"
	"strings"
)

func parseRouterOSVersion(ros string) (uint64, error) {
	parts := strings.Split(strings.TrimSpace(ros), ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid RouterOS version %q: expected major[.minor[.patch]]", ros)
	}
	var version uint64
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, 8)
		if err != nil {
			return 0, fmt.Errorf("invalid RouterOS version %q: %w", ros, err)
		}
		version |= value << uint((2-i)*8)
	}
	if version == 0 {
		return 0, fmt.Errorf("invalid RouterOS version %q: version is zero", ros)
	}
	return version, nil
}

func routerOSVersionAtLeast(minVersion string) (bool, error) {
	if RouterOSVersion == "" {
		return false, fmt.Errorf("RouterOS version is not set")
	}

	currentVersion, err := parseRouterOSVersion(RouterOSVersion)
	if err != nil {
		return false, err
	}

	minimumVersion, err := parseRouterOSVersion(minVersion)
	if err != nil {
		return false, err
	}

	return currentVersion >= minimumVersion, nil
}
