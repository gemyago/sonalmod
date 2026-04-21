package workspacefs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkeletonMarker(t *testing.T) {
	t.Parallel()
	require.Equal(t, "workspacefs-skeleton", SkeletonMarker())
}
