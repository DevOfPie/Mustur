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

// Attach stores an image against a record.
//
// The media type is sniffed from the bytes. A caller's Content-Type is a claim
// about a file the caller also chose, so it is not evidence of anything.
func (s *Store) Attach(ctx context.Context, recordID string, data []byte, by string) (Attachment, error) {
	recordID = strings.ToUpper(strings.TrimSpace(recordID))
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
		strings.ToUpper(strings.TrimSpace(recordID)))
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
