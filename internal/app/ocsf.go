package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/secret"
)

// ocsf.go: the audit trail as OCSF events, for a SIEM (#17).
//
// One class does it: API Activity (6003). A commit to the fleet repository is
// an Update made through the console's API on behalf of its author; revealing
// a device secret is a Read. OCSF has classes that look closer - Account
// Change for an administrator password, File System Activity for a changed
// fleet.json - but each describes the thing on the device or the disk, and
// what the console records is the API call. A SIEM rule then filters on one
// class and the operation.
//
// The events follow the base class without profiles. Every attribute used
// here was checked against OCSF 1.9.0 (schema.ocsf.io): the class's required
// attributes are all present, every name exists in the class or object it is
// set on, and every enum value is one the schema lists.

// OCSFVersion is the schema version the events claim in metadata.version.
const OCSFVersion = "1.9.0"

// OCSFEvent is one API Activity event.
type OCSFEvent struct {
	ActivityID   int            `json:"activity_id"`
	ActivityName string         `json:"activity_name"`
	CategoryUID  int            `json:"category_uid"`
	CategoryName string         `json:"category_name"`
	ClassUID     int            `json:"class_uid"`
	ClassName    string         `json:"class_name"`
	TypeUID      int            `json:"type_uid"`
	TypeName     string         `json:"type_name"`
	SeverityID   int            `json:"severity_id"`
	Severity     string         `json:"severity"`
	StatusID     int            `json:"status_id"`
	Status       string         `json:"status"`
	Time         int64          `json:"time"` // milliseconds since the epoch
	Message      string         `json:"message"`
	Actor        OCSFActor      `json:"actor"`
	API          OCSFAPI        `json:"api"`
	Resources    []OCSFResource `json:"resources"`
	SrcEndpoint  OCSFEndpoint   `json:"src_endpoint"`
	Metadata     OCSFMetadata   `json:"metadata"`
}

// OCSFActor is who acted.
type OCSFActor struct {
	User OCSFUser `json:"user"`
}

// OCSFUser names a person by what the console recorded of them.
type OCSFUser struct {
	Name      string `json:"name"`
	EmailAddr string `json:"email_addr,omitempty"`
}

// OCSFAPI is the call: an operation on a service.
type OCSFAPI struct {
	Operation string      `json:"operation"`
	Service   OCSFService `json:"service"`
}

// OCSFService names the service the operation was made on.
type OCSFService struct {
	Name string `json:"name"`
}

// OCSFResource is what the call touched.
type OCSFResource struct {
	UID  string `json:"uid"`
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

// OCSFEndpoint is where the call came from. The console does not record the
// operator's address, so this names the console: the service that made the
// change on the operator's behalf.
type OCSFEndpoint struct {
	SvcName string `json:"svc_name"`
}

// OCSFMetadata identifies the event and its producer.
type OCSFMetadata struct {
	Version string      `json:"version"`
	UID     string      `json:"uid"`
	LogName string      `json:"log_name"`
	Product OCSFProduct `json:"product"`
}

// OCSFProduct is Sextant.
type OCSFProduct struct {
	Name       string `json:"name"`
	VendorName string `json:"vendor_name"`
	Version    string `json:"version,omitempty"`
}

const (
	ocsfCategoryApplication = 6
	ocsfClassAPIActivity    = 6003
	ocsfActivityRead        = 2
	ocsfActivityUpdate      = 3
	ocsfSeverityInfo        = 1
	ocsfSeverityMedium      = 3
	ocsfStatusSuccess       = 1
)

// OCSF returns the period's audit trail as OCSF events, oldest first: every
// commit to the fleet repository and every reveal of a device secret. It
// reads what Export reads, so it fails where Export fails rather than
// handing a SIEM a period with a hole in it.
func (s *EvidenceService) OCSF(ctx context.Context, from, to time.Time) ([]OCSFEvent, error) {
	ev, err := s.Export(ctx, from, to)
	if err != nil {
		return nil, err
	}
	product := OCSFProduct{Name: "Sextant", VendorName: "DAWO", Version: s.version}
	out := make([]OCSFEvent, 0, len(ev.Commits)+len(ev.Reveals))
	for _, c := range ev.Commits {
		e := ocsfBase(ocsfActivityUpdate, "Update", ocsfSeverityInfo, "Informational", c.When, product)
		e.Message = c.Subject
		e.Actor = OCSFActor{User: OCSFUser{Name: c.Author, EmailAddr: c.Email}}
		e.API = OCSFAPI{Operation: "commit", Service: OCSFService{Name: "fleet repository"}}
		e.Resources = []OCSFResource{{UID: c.Hash, Type: "commit"}}
		e.Metadata.UID = "commit:" + c.Hash
		out = append(out, e)
	}
	for _, r := range ev.Reveals {
		// Medium, not informational: an owner may read a recovery key or an
		// administrator password, and the SIEM should show every time one did.
		e := ocsfBase(ocsfActivityRead, "Read", ocsfSeverityMedium, "Medium", r.At, product)
		e.Message = fmt.Sprintf("%s of %s revealed", secretName(r.Kind), r.Tag)
		e.Actor = OCSFActor{User: ocsfUser(r.By)}
		e.API = OCSFAPI{Operation: "reveal", Service: OCSFService{Name: "device secrets"}}
		e.Resources = []OCSFResource{{UID: r.Tag, Name: string(r.Kind), Type: "device secret"}}
		e.Metadata.UID = fmt.Sprintf("reveal:%s:%s:%d", r.Tag, r.Kind, r.At.UnixNano())
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, nil
}

func ocsfBase(activity int, activityName string, severity int, severityName string, at time.Time, product OCSFProduct) OCSFEvent {
	return OCSFEvent{
		ActivityID:   activity,
		ActivityName: activityName,
		CategoryUID:  ocsfCategoryApplication,
		CategoryName: "Application Activity",
		ClassUID:     ocsfClassAPIActivity,
		ClassName:    "API Activity",
		TypeUID:      ocsfClassAPIActivity*100 + activity,
		TypeName:     "API Activity: " + activityName,
		SeverityID:   severity,
		Severity:     severityName,
		StatusID:     ocsfStatusSuccess,
		Status:       "Success",
		Time:         at.UnixMilli(),
		SrcEndpoint:  OCSFEndpoint{SvcName: "sextant"},
		Metadata:     OCSFMetadata{Version: OCSFVersion, LogName: "audit", Product: product},
	}
}

// ocsfUser fills the address field only when the recorded reader is one: a
// reveal records the e-mail address, or the subject when the identity
// provider gave none.
func ocsfUser(by string) OCSFUser {
	if strings.Contains(by, "@") {
		return OCSFUser{Name: by, EmailAddr: by}
	}
	return OCSFUser{Name: by}
}

func secretName(k secret.Kind) string {
	switch k {
	case secret.LUKS:
		return "LUKS recovery key"
	case secret.Admin:
		return "administrator password"
	}
	return string(k) + " secret"
}
