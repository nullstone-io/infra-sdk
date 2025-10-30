package infra_sdk

const (
	StandardTagStack = "nullstone.io/stack"
	StandardTagEnv   = "nullstone.io/env"
	StandardTagBlock = "nullstone.io/block"
)

// MapStandardTagToLegacy maps the standard tag keys to ones that every Nullstone module contains
// Eventually, the Nullstone modules will use the standard tag keys instead
func MapStandardTagToLegacy(input string) string {
	switch input {
	case StandardTagStack:
		return "Stack"
	case StandardTagEnv:
		return "Env"
	case StandardTagBlock:
		return "Block"
	}
	return input
}

func MapLegacyTagToStandard(input string) string {
	switch input {
	case "Stack":
		return StandardTagStack
	case "Env":
		return StandardTagEnv
	case "Block":
		return StandardTagBlock
	}
	return input
}
