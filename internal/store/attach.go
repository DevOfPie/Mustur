package store

// Images attached to a record, held where the export cannot reach them.
//
// The owner asked for the intake box to take pictures, having tried to report a
// layout defect and found there was no way to send one. The obvious place to
// put them was the exported tree beside the records — and that tree is
// committed to a public repository, so a screenshot of the session view would
// have published agent output, record text and an email address, permanently
// and irreversibly.
//
// The owner's answer was better than any of the options put to them: **the
// description travels and the pixels do not.** An agent reads the image and
// writes what it found into the record's own fields, so a reader with nothing
// but the clone still learns what the picture showed. The bytes stay in the
// store, which never leaves the machine.
//
// Nothing here is exported, and `TestTheExportNeverCarriesImageBytes` is what
// keeps that true rather than this comment.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MaxAttachment is the largest image the intake box will take.
//
// A phone screenshot is one to four megabytes; ten leaves room for a long one
// without letting a single filing become the largest thing in the store.
const MaxAttachment = 10 << 20

// An Attachment is what is known about a stored image without reading it.
type Attachment struct {
	ID        string
	RecordID  string
	MediaType string
	Size      int
	SHA256    string
	Created   time.Time
	CreatedBy string
}

// ErrNotAnImage refuses anything this cannot safely show back.
var ErrNotAnImage = errors.New("that is not an image this will store")

// ErrTooLarge refuses anything past MaxAttachment.
var ErrTooLarge = fmt.Errorf("an image may be at most %d MB", MaxAttachment>>20)

// imageTypes is the whole list, and it is a list rather than a prefix test.
//
// `image/*` would admit SVG, which is XML that can carry script and would run
// on this origin the moment somebody opened it. These four are raster formats a
// browser decodes rather than executes.
var imageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// ImageType decides whether bytes are a picture this will store, and which.
//
// Exported so the decision can be taken before anything is written. It used to
// live inside Attach, which runs after the record exists — so a refused picture
// left a jot behind claiming to have one, under a comment saying it would not.
func ImageType(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrNotAnImage
	}
	kind := http.DetectContentType(data)
	if i := strings.Index(kind, ";"); i >= 0 {
		kind = strings.TrimSpace(kind[:i])
	}
	if !imageTypes[kind] {
		return "", ErrNotAnImage
	}
	return kind, nil
}

// stripMetadata drops everything a camera wrote into a picture that is not the
// picture.
//
// The sender's filename is not stored here, on the reasoning that it carries a
// date, a device and often the content. EXIF carries all three and carries them
// explicitly — the first real upload from the owner's phone arrived naming the
// exact device build it was taken on (MUS-F-0037). The rule was written and
// then applied to the smaller of the two carriers.
//
// Dropped as the bytes are stored rather than as they are served, so a thing
// nobody wanted is never written down at all. Byte surgery rather than a
// re-encode: decoding and re-encoding would lose quality on somebody's evidence
// to remove a header, which is the wrong trade for a screenshot of a defect.
func stripMetadata(kind string, data []byte) []byte {
	switch kind {
	case "image/jpeg":
		return stripJPEG(data)
	case "image/png":
		return stripPNG(data)
	}
	// GIF and WebP carry their own comment and metadata chunks. They are left
	// alone rather than half-handled: an unstripped format named here is a
	// smaller lie than one silently believed to be clean.
	return data
}

// stripJPEG removes the APPn application segments, which is where EXIF, the
// thumbnail, XMP and any colour-profile comment live.
func stripJPEG(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return data
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[0], data[1]) // SOI
	i := 2
	for i+3 < len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		// Start of scan: the compressed image follows and is not segmented.
		if marker == 0xDA {
			return append(out, data[i:]...)
		}
		size := int(data[i+2])<<8 | int(data[i+3])
		if size < 2 || i+2+size > len(data) {
			return data
		}
		// APP0 through APP15, and the comment segment.
		if !(marker >= 0xE0 && marker <= 0xEF) && marker != 0xFE {
			out = append(out, data[i:i+2+size]...)
		}
		i += 2 + size
	}
	if i >= len(data) {
		return out
	}
	return append(out, data[i:]...)
}

// stripPNG removes the ancillary chunks that carry text, time and colour
// profiles, keeping the ones an image needs to decode.
func stripPNG(data []byte) []byte {
	const sig = 8
	if len(data) < sig+12 {
		return data
	}
	drop := map[string]bool{
		"tEXt": true, "iTXt": true, "zTXt": true,
		"tIME": true, "eXIf": true, "iCCP": true,
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:sig]...)
	i := sig
	for i+8 <= len(data) {
		size := int(data[i])<<24 | int(data[i+1])<<16 | int(data[i+2])<<8 | int(data[i+3])
		if size < 0 || i+12+size > len(data) {
			return data
		}
		name := string(data[i+4 : i+8])
		if !drop[name] {
			out = append(out, data[i:i+12+size]...)
		}
		i += 12 + size
		if name == "IEND" {
			break
		}
	}
	return out
}

// Attach stores an image against a record.
//
// The media type is sniffed from the bytes. A caller's Content-Type is a claim
// about a file the caller also chose, so it is not evidence of anything.
//
// The id is taken as given. It used to be upper-cased here, on the reasoning
// that record identifiers are upper-case — which quietly broke the day a
// scratch filing attached a picture: its lower-case id was stored shouting, the
// sweep's subquery no longer matched, and the picture outlived the note it
// belonged to. Callers pass the id they mean; this does not second-guess it.
func (s *Store) Attach(ctx context.Context, recordID string, data []byte, by string) (Attachment, error) {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return Attachment{}, errors.New("an attachment needs a record")
	}
	if len(data) == 0 {
		return Attachment{}, errors.New("that image is empty")
	}
	if len(data) > MaxAttachment {
		return Attachment{}, ErrTooLarge
	}
	kind, err := ImageType(data)
	if err != nil {
		return Attachment{}, err
	}

	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return Attachment{}, err
	}
	// Before the hash, so what is recorded describes what is kept.
	data = stripMetadata(kind, data)
	sum := sha256.Sum256(data)
	a := Attachment{
		ID:        hex.EncodeToString(raw),
		RecordID:  recordID,
		MediaType: kind,
		Size:      len(data),
		SHA256:    hex.EncodeToString(sum[:]),
		Created:   s.now().UTC(),
		CreatedBy: by,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO attachment (id, record_id, media_type, bytes, size, sha256, created, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.RecordID, a.MediaType, data, a.Size, a.SHA256,
		a.Created.Format(time.RFC3339), a.CreatedBy); err != nil {
		return Attachment{}, fmt.Errorf("store attachment: %w", err)
	}
	return a, nil
}

// Attachments lists what is attached to a record, without reading any of it.
func (s *Store) Attachments(ctx context.Context, recordID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, record_id, media_type, size, sha256, created, created_by
		   FROM attachment WHERE record_id = ? ORDER BY created`,
		strings.TrimSpace(recordID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		var a Attachment
		var created string
		if err := rows.Scan(&a.ID, &a.RecordID, &a.MediaType, &a.Size, &a.SHA256, &created, &a.CreatedBy); err != nil {
			return nil, err
		}
		a.Created, _ = time.Parse(time.RFC3339, created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// MoveAttachments hands a record's pictures to the record that replaces it.
//
// A correction files a new record and retires the old one as a stub. The bytes
// stayed behind on the stub, so a jot filed from a phone with a photograph came
// out of a reroute with the photograph attached to the record nobody reads
// (MUS-F-0057). Moved rather than copied: the stub makes no claim any more, and
// a second copy of a 2.4 MB photograph is the wrong price for a correction.
func (s *Store) MoveAttachments(ctx context.Context, from, to string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE attachment SET record_id = ? WHERE record_id = ?`,
		strings.TrimSpace(to), strings.TrimSpace(from))
	if err != nil {
		return 0, fmt.Errorf("move attachments: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// Forget removes an image, leaving the record and its description.
//
// Records are insert-only and an attachment is not a record: it is a private
// thing held beside one, and the durable half was always the description an
// agent wrote from it. So a picture can go — a test filing, a screenshot whose
// job is done, something that should not have been sent — and what it meant
// stays in the record where a reader can still find it.
func (s *Store) Forget(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM attachment WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errors.New("no such attachment")
	}
	return nil
}

// Image reads one back.
func (s *Store) Image(ctx context.Context, id string) (Attachment, []byte, error) {
	var a Attachment
	var created string
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, record_id, media_type, size, sha256, created, created_by, bytes
		   FROM attachment WHERE id = ?`, strings.TrimSpace(id)).
		Scan(&a.ID, &a.RecordID, &a.MediaType, &a.Size, &a.SHA256, &created, &a.CreatedBy, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, nil, errors.New("no such attachment")
	}
	if err != nil {
		return Attachment{}, nil, err
	}
	a.Created, _ = time.Parse(time.RFC3339, created)
	return a, data, nil
}
