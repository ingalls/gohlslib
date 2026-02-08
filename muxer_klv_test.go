package gohlslib

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

func TestMuxerKLV(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{testVideoTrack, klvTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	for i := 0; i < 4; i++ {
		d := time.Duration(i) * time.Second
		pts := int64(d) * 90000 / int64(time.Second)

		err = m.WriteKLV(klvTrack, testTime.Add(d), pts, []byte{
			0x00, 0x01, 0x02, 0x03,
		})
		require.NoError(t, err)

		// Write H264 (IDR to force segment creation)
		err = m.WriteH264(testVideoTrack, testTime.Add(d), pts, [][]byte{
			testSPS,
			{8}, // PPS
			{5}, // IDR
		})
		require.NoError(t, err)
	}

	byts, _, err := doRequest(m, "index.m3u8")
	require.NoError(t, err)

	require.Contains(t, string(byts), "main_stream.m3u8")

	byts, _, err = doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	require.Regexp(t, regexp.MustCompile(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
.*?_main_seg0\.ts
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
.*?_main_seg1\.ts
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
.*?_main_seg2\.ts
`), string(byts))
}
