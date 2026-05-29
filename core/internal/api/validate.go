package api

import (
	"fmt"
	"regexp"

	"arrowhead/core/internal/model"
)

var (
	reSystemName            = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,62}$`)
	reDeviceName            = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,61}[A-Z0-9]$`) // NOTE: single-char device names rejected per AH5 spec regex
	reServiceDefinitionName = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
	reInterfaceTemplateName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

func validateSystemName(name string) string {
	if !reSystemName.MatchString(name) {
		return "systemName must be PascalCase (^[A-Z][A-Za-z0-9]{0,62}$), got: " + name
	}
	return ""
}

func validateDeviceName(name string) string {
	if !reDeviceName.MatchString(name) {
		return "deviceName must be UPPER_SNAKE_CASE with no trailing underscore " +
			"(^[A-Z][A-Z0-9_]{0,61}[A-Z0-9]$), got: " + name
	}
	return ""
}

func validateServiceDefinitionName(name string) string {
	if !reServiceDefinitionName.MatchString(name) {
		return "serviceDefinitionName must be camelCase (^[a-z][A-Za-z0-9]{0,62}$), got: " + name
	}
	return ""
}

func validateInterfaceTemplateName(name string) string {
	if !reInterfaceTemplateName.MatchString(name) {
		return "interfaceTemplateName must be snake_case (^[a-z][a-z0-9_]{0,62}$), got: " + name
	}
	return ""
}

// validateInterfaces checks each InterfaceInstance for a valid SecurityPolicy.
// Returns a non-empty error message if any interface has an invalid policy.
func validateInterfaces(interfaces []model.InterfaceInstance) string {
	for _, intf := range interfaces {
		if intf.Policy != "" && !model.IsValidSecurityPolicy(intf.Policy) {
			return fmt.Sprintf("invalid policy %q: must be one of NONE, CERT_AUTH, TIME_LIMITED_TOKEN_AUTH, USAGE_LIMITED_TOKEN_AUTH, BASE64_SELF_CONTAINED_TOKEN_AUTH, RSA_SHA256_JSON_WEB_TOKEN_AUTH, RSA_SHA512_JSON_WEB_TOKEN_AUTH", intf.Policy)
		}
	}
	return ""
}

// hasServiceLookupFilter returns true if at least one primary filter is set.
func hasServiceLookupFilter(req model.ServiceLookupRequest) bool {
	return len(req.InstanceIDs) > 0 || len(req.ProviderNames) > 0 || len(req.ServiceDefinitionNames) > 0
}
