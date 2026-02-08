package gohlslib

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

func TestMuxerKLV(t *testing.T) {
	track := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{track},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 4; i++ {
		err = m.WriteKLV(track, testTime.Add(time.Duration(i)*time.Second), int64(i)*90000, []byte{
			0x00, 0x01, 0x02, 0x03,
		})
		require.NoError(t, err)
	}

	// check primary playlist
	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:3\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=200,AVERAGE-BANDWIDTH=200\n"+
		"main_stream.m3u8\n", string(byts))

	// check stream playlist
	byts, _, err = doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	require.Regexp(t, regexp.MustCompile(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:1.00000,
main_stream_seg0.ts
#EXTINF:1.00000,
main_stream_seg1.ts
#EXTINF:1.00000,
main_stream_seg2.ts
`), string(byts))
}
