package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
	"github.com/bhouse-nexthop/openpsirt/internal/version"
)

// Ingest is what the upload endpoint needs to do its job.
type Ingest struct {
	DB     *database.DB
	Queue  *queue.Queue
	Limits sbom.Limits
	// Access resolves who is asking. A nil resolver means nothing is
	// authorized, which is what a process that cannot tell who is asking
	// should answer.
	Access *access.Resolver
}

// catalog returns a store over this deployment's database, or nothing when
// there is no database — which is the process that only renders the API
// document.
func (in Ingest) catalog() *catalog.Store {
	if in.DB == nil {
		return nil
	}
	return catalog.NewStore(in.DB.DB)
}

// uploadParts are the documents a build sends.
//
// One request carries the whole picture. A build whose inventory landed and
// whose suppressions did not would have every carried patch reported as an
// outstanding vulnerability, which is worse than the upload having failed.
type uploadParts struct {
	// Inventory is what the build shipped.
	//
	// The declared content type is deliberately permissive. What a part is
	// gets decided by reading it, not by the label a client put on it — and
	// the labels vary: a build pushing a file with an ordinary command-line
	// client sends it as opaque bytes, which is not wrong and is not worth
	// refusing an otherwise good scan over.
	Inventory huma.FormFile `form:"inventory" contentType:"application/json,application/octet-stream" required:"true"`
	// Suppressions are what the build has already argued does not apply to
	// it. A build's suppressions are a directory rather than a file, so this
	// repeats — and a build that carries no patches sends none, so it is
	// optional.
	Suppressions []huma.FormFile `form:"suppressions" contentType:"application/json,application/octet-stream" required:"false"`
}

// UploadInput is an arriving scan.
type UploadInput struct {
	Product string `path:"product" doc:"The declared product this build is of"`
	Stream  string `path:"stream" doc:"The declared branch or tag"`
	Variant string `path:"variant" doc:"The declared way that stream is built"`
	RawBody huma.MultipartFormFiles[uploadParts]
}

// UploadOutput reports what became of it.
type UploadOutput struct {
	Status int
	Body   UploadResult
}

// UploadResult is what a producer gets back.
type UploadResult struct {
	ScanID int64 `json:"scan_id" doc:"The scan this upload became, or the one it matched"`
	// Outcome says what happened, in the producer's terms rather than ours.
	Outcome string `json:"outcome" enum:"queued,already_held" doc:"Whether this upload was taken or matched one already held"`
	Serial  string `json:"serial,omitempty" doc:"The identity the inventory carries for itself"`
	BuiltAt string `json:"built_at,omitempty" doc:"When the producer says the build was made"`
}

func registerScans(api huma.API, in Ingest) {
	// Registered whether or not there is a database behind it. The OpenAPI
	// document is generated from these registrations by a process that never
	// opens one, and an operation missing from the document because of how it
	// was generated is exactly the drift generating it is meant to prevent.
	huma.Register(api, huma.Operation{
		OperationID: "upload-scan",
		Method:      http.MethodPost,
		Path:        "/v1/products/{product}/streams/{stream}/variants/{variant}/scans",
		Summary:     "Send what a build shipped",
		Description: "Takes a build's inventory and the suppressions it carries, against a product, " +
			"stream and variant that have already been declared. The documents are read after the " +
			"response, so a successful reply means they were accepted rather than that they parsed.",
		Tags: []string{"Ingest"},
		// A scan file is somebody else's output arriving over a link we do not
		// control, and this is the first place it can be stopped.
		MaxBodyBytes:  maxUpload(in.Limits),
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, input *UploadInput) (*UploadOutput, error) {
		return upload(ctx, in, input)
	})
}

// maxUpload bounds a whole request, which is more than one document.
//
// A build sends an inventory and however many suppression documents it has, so
// the request ceiling is not the document ceiling. Twice leaves room for the
// suppressions without letting a request be unbounded.
func maxUpload(limits sbom.Limits) int64 {
	return 2 * limits.OrDefault().MaxBytes
}

func upload(ctx context.Context, in Ingest, input *UploadInput) (*UploadOutput, error) {
	if in.DB == nil || in.Queue == nil {
		return nil, huma.Error500InternalServerError("this process cannot take uploads")
	}
	parts := input.RawBody.Data()

	// Refusing before reading. Deciding first costs a query; deciding after
	// costs however long it takes to store tens of megabytes we then throw
	// away, on a deployment already behind on its work.
	depth, err := in.Queue.Depth(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot tell how much work is waiting", err)
	}
	if depth >= in.Queue.MaxBacklog() {
		return nil, huma.NewError(http.StatusServiceUnavailable,
			fmt.Sprintf("%d scans are already waiting to be read; try again shortly", depth))
	}

	target, err := catalog.NewStore(in.DB.DB).Resolve(ctx, input.Product, input.Stream, input.Variant)
	if err != nil {
		// The message names which part is missing, which is what whoever sees
		// the failed upload needs in order to declare it.
		return nil, huma.Error404NotFound(err.Error())
	}

	// One pass over the inventory answers both questions asked of an arriving
	// scan: what it is, and whether we already hold it.
	header, contentHash, err := describe(parts.Inventory, in.Limits)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("the inventory could not be read", err)
	}

	// A document that does not say when it was built cannot be ordered against
	// anything, and taking it is worse than refusing it: the zero time is
	// older than every real one, so the first such upload is accepted and
	// every later scan for that target is refused as not newer. The target
	// takes no further scans at all, which is the same wedge the future-clock
	// check exists to prevent, arriving through a door nobody guarded.
	if header.BuiltAt.IsZero() {
		return nil, huma.Error400BadRequest(
			"the inventory does not say when it was built, and that is what orders scans against each other")
	}

	arriving := ingest.Arriving{
		TargetID:      target.ID,
		ContentHash:   contentHash,
		Serial:        header.Serial,
		BuiltAt:       header.BuiltAt,
		ParserVersion: version.Get().Version,
	}

	var (
		result  UploadResult
		outcome ingest.Outcome
	)
	err = in.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		scan, taken, err := ingest.NewStore(tx).Record(ctx, arriving)
		// Recorded before the error is checked: a refusal carries the reason
		// in the outcome, and that is what decides which answer the producer
		// gets back.
		outcome = taken
		if err != nil {
			return err
		}
		result = UploadResult{ScanID: scan.ID, Serial: header.Serial}
		if !header.BuiltAt.IsZero() {
			result.BuiltAt = header.BuiltAt.UTC().Format(time.RFC3339)
		}
		if taken != ingest.Accept {
			return nil
		}

		// Everything from here commits together. A scan row without its
		// documents is unreadable, documents without a job are work nobody
		// picks up, and a job without either is a worker failing on something
		// that was never there.
		documents := ingest.NewDocuments(tx)
		if err := store(ctx, documents, scan.ID, ingest.InventoryKind, 0, parts.Inventory); err != nil {
			return err
		}
		for i, part := range parts.Suppressions {
			if !part.IsSet {
				continue
			}
			if err := store(ctx, documents, scan.ID, ingest.SuppressionsKind, i, part); err != nil {
				return err
			}
		}
		_, err = in.Queue.AddTx(ctx, tx, ingest.JobKind, fmt.Sprint(scan.ID))
		return err
	})

	switch {
	case errors.Is(err, queue.ErrBacklogFull):
		return nil, huma.NewError(http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ingest.ErrRejected):
		return nil, rejection(outcome, err)
	case err != nil:
		return nil, huma.Error500InternalServerError("the upload could not be recorded", err)
	}

	out := &UploadOutput{Status: http.StatusAccepted, Body: result}
	if outcome == ingest.AlreadyHave {
		// Answered with success, not an error. The ordinary case is a retry
		// after a timeout that had in fact succeeded, and failing it turns a
		// landed scan into a red build.
		out.Status = http.StatusOK
		out.Body.Outcome = "already_held"
		return out, nil
	}
	out.Body.Outcome = "queued"
	return out, nil
}

// rejection turns a refusal into the status that describes it.
func rejection(outcome ingest.Outcome, err error) error {
	if outcome == ingest.BuiltInFuture {
		// The producer's own clock is wrong, which is a fault in the request
		// rather than a conflict with anything we hold.
		return huma.Error400BadRequest(err.Error())
	}
	return huma.NewError(http.StatusConflict, err.Error())
}

// describe reads what an inventory says about itself, and hashes it.
//
// Both in one pass: the file is seekable, but a second pass over tens of
// megabytes buys nothing.
func describe(file huma.FormFile, limits sbom.Limits) (sbom.Header, string, error) {
	if !file.IsSet {
		return sbom.Header{}, "", fmt.Errorf("no inventory was sent")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return sbom.Header{}, "", err
	}
	digest := sha256.New()
	header, err := sbom.ReadHeader(io.TeeReader(file, digest), limits)
	if err != nil {
		return sbom.Header{}, "", err
	}
	// The header may be answered before the end of the file, and the hash has
	// to cover all of it.
	if _, err := io.Copy(digest, file); err != nil {
		return sbom.Header{}, "", err
	}
	return header, hex.EncodeToString(digest.Sum(nil)), nil
}

// store rewinds a part and puts it away.
func store(ctx context.Context, documents *ingest.Documents, scanID int64, kind ingest.Kind, ordinal int, file huma.FormFile) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := documents.Write(ctx, scanID, kind, ordinal, file)
	return err
}
