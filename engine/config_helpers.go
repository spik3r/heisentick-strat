package engine

import "github.com/spik3r/heisentick-strat/dsl"

func setupTypeFromAny(value any) string {
	switch setupType := value.(type) {
	case string:
		return setupType
	case dsl.FamilyID:
		return string(setupType)
	default:
		return ""
	}
}
