package pcap_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brimsec/zq/pcap"
	"github.com/stretchr/testify/require"
)

func TestInvalidIndex(t *testing.T) {
	r := strings.NewReader("this is not a valid pcap")
	_, err := pcap.CreateIndex(r, 0)
	require.Error(t, err)
	var e *pcap.ErrInvalidPcap
	if !errors.Is(err, e) {
		require.FailNow(t, "error is not of type pcap.ErrInvalidPcap", err)
	}
}
