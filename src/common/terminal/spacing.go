package terminal

// Consistent spacing units for TUI layout (PART 32, AI.md "Spacing and
// Alignment", line 44529).
const (
	// SpaceXS is micro spacing.
	SpaceXS = 1
	// SpaceS is small spacing.
	SpaceS = 2
	// SpaceM is medium spacing.
	SpaceM = 4
	// SpaceL is large spacing.
	SpaceL = 6
	// SpaceXL is extra large spacing.
	SpaceXL = 8
)

// GetSpacingForMode applies spacing based on the terminal size mode.
func GetSpacingForMode(m SizeMode) int {
	switch m {
	case SizeModeMicro, SizeModeMinimal:
		return SpaceXS
	case SizeModeCompact:
		return SpaceS
	case SizeModeStandard:
		return SpaceM
	case SizeModeWide:
		return SpaceL
	// Ultrawide, Massive
	default:
		return SpaceXL
	}
}
