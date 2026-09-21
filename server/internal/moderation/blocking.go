package moderation

import "fmt"

// Blocking (master-plan 32).
//
// A listener who is being harassed has exactly one tool without this: file a
// report and wait for a human to agree. A block is the other tool — a
// self-service boundary that takes effect immediately, requires nobody's
// agreement, and is reversible by the person who set it.
//
// What a block is *not*: it is not a moderation action. It does not delete
// anything, it does not penalise the blocked account, and it is not visible to
// them. It changes what one account is served. Treating it as punishment is how
// block features become harassment tools in the other direction, and it is why
// the effects below are all defined as filters on the blocker's own view.

// BlockReasonMaxLen bounds the optional note. It is stored for the blocker's
// own reference and for a moderator reading a later report; it is not shown to
// the blocked account.
const BlockReasonMaxLen = 500

// ValidateBlock checks the two ids a block names.
//
// Self-blocking is refused rather than accepted as a no-op. A listener who
// reaches for "block" on their own account is almost certainly confused about
// what the button does, and silently creating a row that filters their own
// content out of their own feed would be the worst available answer.
func ValidateBlock(blockerID, blockedID string) error {
	if blockerID == "" || blockedID == "" {
		return fmt.Errorf("a block needs both a blocker and a blocked account")
	}
	if blockerID == blockedID {
		return fmt.Errorf("an account cannot block itself")
	}
	return nil
}

// ValidateBlockReason bounds the optional note. An empty reason is valid:
// most blocks are not explained and do not need to be.
func ValidateBlockReason(reason string) error {
	if len([]rune(reason)) > BlockReasonMaxLen {
		return fmt.Errorf("block reason must be at most %d characters", BlockReasonMaxLen)
	}
	return nil
}

// BlocksReaction reports whether a reaction between two accounts is prevented
// by a block in either direction.
//
// Both directions matter and they are not the same case. If I blocked you, your
// reaction on my post reaching me is exactly what the block was for. If you
// blocked me, my reaction on your post is unwanted contact with someone who
// asked not to hear from me, and honouring only one direction would make the
// block half a boundary.
func BlocksReaction(blockerBlockedMe, iBlockedThem bool) bool {
	return blockerBlockedMe || iBlockedThem
}
