package catalog

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("resource not found")

const (
	MinimumResourceID int64 = 1
)

type PageRequest struct {
	AfterID int64
	Size    int
}

func (p PageRequest) FetchSize() int {
	return p.Size + 1
}

type PageItem interface {
	pageID() int64
}

type Page[T PageItem] struct {
	Items   []T
	HasMore bool
}

type HashPageRequest struct {
	AfterHash *int32
	Size      int
}

func (p HashPageRequest) FetchSize() int {
	return p.Size + 1
}

type HashPageItem interface {
	pageHash() int32
}

type HashPage[T HashPageItem] struct {
	Items   []T
	HasMore bool
}

func NewHashPage[T HashPageItem](items []T, requestedSize int) HashPage[T] {
	hasMore := len(items) > requestedSize
	if hasMore {
		items = items[:requestedSize]
	}
	return HashPage[T]{Items: items, HasMore: hasMore}
}

func (p HashPage[T]) NextAfterHash() *int32 {
	if !p.HasMore || len(p.Items) == 0 {
		return nil
	}
	next := p.Items[len(p.Items)-1].pageHash()
	return &next
}

func NewPage[T PageItem](items []T, requestedSize int) Page[T] {
	hasMore := len(items) > requestedSize
	if hasMore {
		items = items[:requestedSize]
	}
	return Page[T]{Items: items, HasMore: hasMore}
}

func (p Page[T]) NextAfterID() *int64 {
	if !p.HasMore || len(p.Items) == 0 {
		return nil
	}
	next := p.Items[len(p.Items)-1].pageID()
	return &next
}

type ArtistFilter struct {
	Name     string
	RealName string
}

type LabelFilter struct {
	Name string
}

type MasterFilter struct {
	Title string
	Year  *int
}

type ReleaseFilter struct {
	Title   string
	Country string
	Year    *int
	Month   *int
	Master  *bool
}

type CatalogNumberLookup struct {
	LabelID       int64
	CatalogNumber string
}

type IdentifierLookup struct {
	Type  string
	Value string
}

type SnapshotReader interface {
	Snapshot(context.Context) (CatalogSnapshot, error)
}

type ArtistReader interface {
	SearchArtists(context.Context, ArtistFilter, PageRequest) (Page[Artist], error)
	Artist(context.Context, int64) (ArtistDetail, error)
	ArtistReleases(context.Context, int64, PageRequest) (Page[ArtistRelease], error)
}

type LabelReader interface {
	SearchLabels(context.Context, LabelFilter, PageRequest) (Page[Label], error)
	Label(context.Context, int64) (LabelDetail, error)
	LabelReleases(context.Context, int64, PageRequest) (Page[LabelRelease], error)
}

type MasterReader interface {
	SearchMasters(context.Context, MasterFilter, PageRequest) (Page[Master], error)
	Master(context.Context, int64) (MasterDetail, error)
	MasterReleases(context.Context, int64, PageRequest) (Page[MasterRelease], error)
}

type ReleaseReader interface {
	SearchReleases(context.Context, ReleaseFilter, PageRequest) (Page[Release], error)
	ReleasesByCatalogNumber(context.Context, CatalogNumberLookup, PageRequest) (Page[Release], error)
	ReleasesByIdentifier(context.Context, IdentifierLookup, PageRequest) (Page[Release], error)
	Release(context.Context, int64) (ReleaseDetail, error)
	ReleaseTracks(context.Context, int64, HashPageRequest) (HashPage[ReleaseTrack], error)
	ReleaseIdentifiers(context.Context, int64, HashPageRequest) (HashPage[ReleaseIdentifier], error)
}

type Repository interface {
	SnapshotReader
	ArtistReader
	LabelReader
	MasterReader
	ReleaseReader
}

type CatalogSnapshot struct {
	Ready     bool             `json:"ready"`
	Status    string           `json:"status"`
	UpdatedAt time.Time        `json:"updated_at"`
	Entities  []SnapshotEntity `json:"entities"`
}

type SnapshotEntity struct {
	EntityType       string     `json:"entity_type"`
	Status           string     `json:"status"`
	DumpDate         *string    `json:"dump_date"`
	AppliedAt        *time.Time `json:"applied_at"`
	Processor        *string    `json:"processor"`
	ProcessorVersion *string    `json:"processor_version"`
}

type Artist struct {
	ID          int64   `json:"id"`
	Name        *string `json:"name"`
	RealName    *string `json:"real_name"`
	Profile     *string `json:"profile"`
	DataQuality *string `json:"data_quality"`
}

func (item Artist) pageID() int64 { return item.ID }

type ArtistReference struct {
	ID          int64   `json:"id"`
	Name        *string `json:"name"`
	ResourceURL string  `json:"resource_url"`
}

type ArtistDetail struct {
	Artist
	Members        []ArtistReference `json:"members"`
	Groups         []ArtistReference `json:"groups"`
	Aliases        []ArtistReference `json:"aliases"`
	NameVariations []string          `json:"namevariations"`
	URLs           []string          `json:"urls"`
	URI            string            `json:"uri"`
	ReleaseURL     string            `json:"release_url"`
}

type ArtistRelease struct {
	ID                int64   `json:"id"`
	Role              *string `json:"role"`
	Title             *string `json:"title"`
	Country           *string `json:"country"`
	DataQuality       *string `json:"data_quality"`
	ReleasedYear      *int    `json:"released_year"`
	ReleasedMonth     *int    `json:"released_month"`
	ReleasedDay       *int    `json:"released_day"`
	ListedReleaseDate *string `json:"listed_release_date"`
	IsMaster          *bool   `json:"is_master"`
	MasterID          *int64  `json:"master_id"`
	Notes             *string `json:"notes"`
	Status            *string `json:"status"`
	ResourceURL       string  `json:"resource_url"`
}

func (item ArtistRelease) pageID() int64 { return item.ID }

type Label struct {
	ID          int64   `json:"id"`
	ContactInfo *string `json:"contact_info"`
	DataQuality *string `json:"data_quality"`
	Name        *string `json:"name"`
	Profile     *string `json:"profile"`
}

func (item Label) pageID() int64 { return item.ID }

type LabelReference struct {
	ID          int64   `json:"id"`
	Name        *string `json:"name"`
	ResourceURL string  `json:"resource_url"`
}

type LabelDetail struct {
	Label
	ParentLabel *LabelReference  `json:"parent_label"`
	Sublabels   []LabelReference `json:"sublabels"`
	URLs        []string         `json:"urls"`
	URI         string           `json:"uri"`
	ReleaseURL  string           `json:"release_url"`
}

type LabelRelease struct {
	ID             int64    `json:"id"`
	Artist         *string  `json:"artist"`
	Title          *string  `json:"title"`
	Year           *int     `json:"year"`
	Status         *string  `json:"status"`
	CatalogNumbers []string `json:"catnos"`
	Format         *string  `json:"format"`
}

func (item LabelRelease) pageID() int64 { return item.ID }

type Master struct {
	ID           int64   `json:"id"`
	DataQuality  *string `json:"data_quality"`
	Title        *string `json:"title"`
	ReleasedYear *int    `json:"released_year"`
}

func (item Master) pageID() int64 { return item.ID }

type MasterVideo struct {
	URL         *string `json:"url"`
	Description *string `json:"description"`
	Title       *string `json:"title"`
}

type MasterDetail struct {
	ID          int64             `json:"id"`
	Title       *string           `json:"title"`
	DataQuality *string           `json:"data_quality"`
	MainRelease *int64            `json:"main_release"`
	Year        *int              `json:"year"`
	Genres      []string          `json:"genres"`
	Styles      []string          `json:"styles"`
	Artists     []ArtistReference `json:"artists"`
	Videos      []MasterVideo     `json:"videos"`
}

type MasterRelease struct {
	ID          int64    `json:"id"`
	Title       *string  `json:"title"`
	Artists     []string `json:"artist"`
	ArtistIDs   []int64  `json:"artist_id"`
	Year        *int     `json:"year"`
	ResourceURL string   `json:"resource_url"`
}

func (item MasterRelease) pageID() int64 { return item.ID }

type Release struct {
	ID                int64   `json:"id"`
	Title             *string `json:"title"`
	Country           *string `json:"country"`
	DataQuality       *string `json:"data_quality"`
	ReleasedYear      *int    `json:"released_year"`
	ReleasedMonth     *int    `json:"released_month"`
	ReleasedDay       *int    `json:"released_day"`
	ListedReleaseDate *string `json:"listed_release_date"`
	IsMaster          *bool   `json:"is_master"`
	MasterID          *int64  `json:"master_id"`
	Notes             *string `json:"notes"`
	Status            *string `json:"status"`
}

func (item Release) pageID() int64 { return item.ID }

type ReleaseArtist struct {
	ID          int64   `json:"id"`
	Name        *string `json:"name"`
	Role        *string `json:"role"`
	ResourceURL string  `json:"resource_url"`
}

type ReleaseLabel struct {
	ID               int64   `json:"id"`
	Name             *string `json:"name"`
	CategoryNotation *string `json:"catno"`
	ResourceURL      string  `json:"resource_url"`
}

type ReleaseFormat struct {
	Name         *string  `json:"name"`
	Quantity     *string  `json:"qty"`
	Descriptions []string `json:"descriptions"`
}

type ReleaseVideo struct {
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	Description *string `json:"description"`
}

type ReleaseTrack struct {
	Hash     int32   `json:"-"`
	Duration *string `json:"duration"`
	Position *string `json:"position"`
	Title    *string `json:"title"`
}

func (item ReleaseTrack) pageHash() int32 { return item.Hash }

type ReleaseIdentifier struct {
	Hash        int32   `json:"-"`
	Description *string `json:"description"`
	Type        *string `json:"type"`
	Value       *string `json:"value"`
}

func (item ReleaseIdentifier) pageHash() int32 { return item.Hash }

type ReleaseDetail struct {
	Release
	Artists   []ReleaseArtist `json:"artists"`
	Labels    []ReleaseLabel  `json:"labels"`
	Companies []ReleaseLabel  `json:"companies"`
	Formats   []ReleaseFormat `json:"formats"`
	Styles    []string        `json:"styles"`
	Genres    []string        `json:"genres"`
	Videos    []ReleaseVideo  `json:"videos"`
}
