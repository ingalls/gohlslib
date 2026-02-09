package gohlslib

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
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

	// Parse the multivariant playlist to verify CODECS attribute remains valid with KLV
	var mvPlaylist playlist.Multivariant
	err = mvPlaylist.Unmarshal(byts)
	require.NoError(t, err, "Failed to parse multivariant playlist")

	// Verify that we have at least one variant
	require.NotEmpty(t, mvPlaylist.Variants, "Expected at least one variant in multivariant playlist")

	// Verify CODECS attribute is well-formed and doesn't contain empty strings
	for _, variant := range mvPlaylist.Variants {
		require.NotEmpty(t, variant.Codecs, "CODECS attribute should not be empty")
		for _, codec := range variant.Codecs {
			require.NotEmpty(t, codec, "CODECS attribute should not contain empty strings")
		}
	}

	// Verify the marshaled playlist doesn't have malformed CODECS
	marshaled, err := mvPlaylist.Marshal()
	require.NoError(t, err)
	marshaledStr := string(marshaled)
	require.NotContains(t, marshaledStr, "CODECS=\",", "CODECS should not start with a comma")
	require.NotContains(t, marshaledStr, ",,", "CODECS should not contain double commas")
	require.NotContains(t, marshaledStr, ",\"", "CODECS should not end with a comma before closing quote")

	byts, _, err = doRequest(m, "main_stream.m3u8")
	require.NoError(t, err)

	re := regexp.MustCompile(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg0\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg1\.ts)
#EXT-X-PROGRAM-DATE-TIME:.*?
#EXTINF:1.00000,
(.*?_main_seg2\.ts)
`)
	require.Regexp(t, re, string(byts))
	ma := re.FindStringSubmatch(string(byts))

	// Fetch the first segment and parse it to verify KLV data
	segmentData, _, err := doRequest(m, ma[1])
	require.NoError(t, err)
	require.NotEmpty(t, segmentData)

	// Parse the MPEG-TS segment using astits
	demuxer := astits.NewDemuxer(context.Background(), bytes.NewReader(segmentData))

	// Track whether we found KLV data
	foundKLVPID := false
	foundKLVData := false
	var klvPID uint16
	var receivedKLVData []byte

	// Iterate through all packets in the segment
	for {
		data, err := demuxer.NextData()
		if err != nil {
			break
		}

		// Check if this is a PMT (Program Map Table) and find KLV PID
		if data.PMT != nil && !foundKLVPID {
			for _, es := range data.PMT.ElementaryStreams {
				// KLV data uses stream type 0x06 (private data)
				if es.StreamType == astits.StreamTypePrivateData {
					klvPID = es.ElementaryPID
					foundKLVPID = true
					break
				}
			}
		}

		// Check if this packet contains KLV data
		if foundKLVPID && data.PES != nil && data.PID == klvPID && !foundKLVData {
			foundKLVData = true
			receivedKLVData = data.PES.Data
			// Verify the KLV data matches what we wrote
			require.Equal(t, []byte{0x00, 0x01, 0x02, 0x03}, receivedKLVData,
				"KLV data content mismatch")
			break
		}
	}

	require.True(t, foundKLVPID, "KLV PID was not found in PMT")
	require.True(t, foundKLVData, "KLV data was not found in the segment")
}

func TestMuxerKLVOnlyTrackRejected(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack},
	}

	err := m.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "KLV tracks require at least one video or audio track")
}

func TestMuxerKLVFirstTrackWithAudio(t *testing.T) {
	klvTrack := &Track{
		Codec:     &codecs.KLV{},
		ClockRate: 90000,
	}

	audioTrack := &Track{
		Codec: &codecs.MPEG4Audio{
			Config: mpeg4audio.AudioSpecificConfig{
				Type:          2,
				SampleRate:    44100,
				ChannelConfig: 2,
				ChannelCount:  2,
			},
		},
		ClockRate: 44100,
	}

	m := &Muxer{
		Variant:            MuxerVariantMPEGTS,
		SegmentCount:       3,
		SegmentMinDuration: 1 * time.Second,
		Tracks:             []*Track{klvTrack, audioTrack},
	}

	err := m.Start()
	require.NoError(t, err)
	defer m.Close()

	// Verify that the audio track is the leading track, not KLV
	require.False(t, m.mtracksByTrack[klvTrack].isLeading, "KLV track should not be leading")
	require.True(t, m.mtracksByTrack[audioTrack].isLeading, "Audio track should be leading")
}
