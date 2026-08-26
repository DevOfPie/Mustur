package store

// What a picture loses on the way in.
//
// The sender's filename is never stored, on the reasoning that it carries a
// date, a device and often the content. The first real upload from the owner's
// phone arrived carrying EXIF that named the exact device build — all three
// things, more explicitly, in the half of the file nobody had looked at
// (MUS-F-0037).

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"
)

func sample(t *testing.T, as string) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for x := 0; x < 32; x++ {
		for y := 0; y < 24; y++ {
			m.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 10), B: 0x30, A: 0xff})
		}
	}
	var buf bytes.Buffer
	var err error
	if as == "jpeg" {
		err = jpeg.Encode(&buf, m, nil)
	} else {
		err = png.Encode(&buf, m)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withEXIF splices an APP1 Exif segment in after the SOI, which is where a
// phone puts one.
func withEXIF(t *testing.T, base []byte, secret string) []byte {
	t.Helper()
	payload := append([]byte("Exif\x00\x00"), []byte(secret)...)
	size := len(payload) + 2
	seg := []byte{0xFF, 0xE1, byte(size >> 8), byte(size)}
	seg = append(seg, payload...)
	out := append([]byte{}, base[:2]...)
	out = append(out, seg...)
	return append(out, base[2:]...)
}

func TestAPictureLosesItsMetadataOnTheWayIn(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const device = "Android BP4A.251205.006.SOMEPHONE"
	carrying := withEXIF(t, sample(t, "jpeg"), device)
	if !bytes.Contains(carrying, []byte(device)) {
		t.Fatal("the sample carries nothing to strip, so this would prove nothing")
	}

	a, err := s.Attach(ctx, "MUS-F-0001", carrying, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, kept, err := s.Image(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(kept, []byte(device)) {
		t.Error("the device name is in the store; a filename was refused for carrying less than this")
	}
	if bytes.Contains(kept, []byte("Exif")) {
		t.Error("the Exif block survived")
	}
	// Still a picture. Stripping a header must not cost the evidence.
	img, err := jpeg.Decode(bytes.NewReader(kept))
	if err != nil {
		t.Fatalf("the stripped image no longer decodes: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 24 {
		t.Errorf("the image changed size to %v", img.Bounds().Size())
	}
	// And the recorded size describes what is actually held.
	if a.Size != len(kept) {
		t.Errorf("the record says %d bytes and the store holds %d", a.Size, len(kept))
	}
}

func TestAPNGLosesItsTextChunks(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// A tEXt chunk, spliced after the header, as a screenshot tool writes one.
	base := sample(t, "png")
	at := bytes.Index(base, []byte("IDAT")) - 4
	if at < 0 {
		t.Fatal("no IDAT in the sample")
	}
	const secret = "SoftwareSomeCaptureTool 4.2 on a named device"
	chunk := []byte{0, 0, 0, byte(len(secret))}
	chunk = append(chunk, []byte("tEXt")...)
	chunk = append(chunk, []byte(secret)...)
	chunk = append(chunk, 0, 0, 0, 0) // CRC, not checked by the stripper
	carrying := append(append(append([]byte{}, base[:at]...), chunk...), base[at:]...)

	a, err := s.Attach(ctx, "MUS-F-0001", carrying, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, kept, err := s.Image(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(kept, []byte(secret)) {
		t.Error("the text chunk survived into the store")
	}
	if _, err := png.Decode(bytes.NewReader(kept)); err != nil {
		t.Fatalf("the stripped image no longer decodes: %v", err)
	}
}

// A picture can be dropped without taking the record with it: the description
// an agent wrote from it is the half that was meant to last.
func TestForgettingAPictureKeepsTheRecord(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a, err := s.Attach(ctx, "MUS-F-0001", sample(t, "png"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Image(ctx, a.ID); err == nil {
		t.Error("the picture is still readable after being forgotten")
	}
	if err := s.Forget(ctx, a.ID); err == nil {
		t.Error("forgetting it twice reported success")
	}
	left, err := s.Attachments(ctx, "MUS-F-0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("%d attachments still listed", len(left))
	}
}
