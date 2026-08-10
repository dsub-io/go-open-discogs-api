package catalog

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("resource not found")

type Direction string

const (
	Ascending          Direction = "asc"
	Descending         Direction = "desc"
	FieldID                      = "id"
	FieldName                    = "name"
	FieldRealName                = "real_name"
	FieldProfile                 = "profile"
	FieldContactInfo             = "contact_info"
	FieldDataQuality             = "data_quality"
	FieldTitle                   = "title"
	FieldCountry                 = "country"
	FieldMasterID                = "master_id"
	FieldReleasedYear            = "released_year"
	FieldReleasedMonth           = "released_month"
	FieldReleasedDay             = "released_day"
	FieldYear                    = "year"
)

type Sort struct {
	Field     string
	Direction Direction
}

type PageRequest struct {
	Page int
	Size int
	Sort []Sort
}

func (p PageRequest) Offset() int64 {
	return int64(p.Page-1) * int64(p.Size)
}

type PageItem interface {
	pageItem()
}

type Page[T PageItem] struct {
	Items []T
	Total int64
}

type ArtistFilter struct {
	Name     string
	RealName string
	Profile  string
}

type LabelFilter struct {
	ContactInfo string
	DataQuality string
	Name        string
	Profile     string
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
	Release(context.Context, int64) (ReleaseDetail, error)
}

type Repository interface {
	ArtistReader
	LabelReader
	MasterReader
	ReleaseReader
}

type Artist struct {
	ID          int64   `json:"id"`
	Name        *string `json:"name"`
	RealName    *string `json:"real_name"`
	Profile     *string `json:"profile"`
	DataQuality *string `json:"data_quality"`
}

func (Artist) pageItem() {}

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

func (ArtistRelease) pageItem() {}

type Label struct {
	ID          int64   `json:"id"`
	ContactInfo *string `json:"contact_info"`
	DataQuality *string `json:"data_quality"`
	Name        *string `json:"name"`
	Profile     *string `json:"profile"`
}

func (Label) pageItem() {}

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
	ID               int64   `json:"id"`
	Artist           *string `json:"artist"`
	Title            *string `json:"title"`
	Year             *int    `json:"year"`
	Status           *string `json:"status"`
	CategoryNotation *string `json:"catno"`
	Format           *string `json:"format"`
}

func (LabelRelease) pageItem() {}

type Master struct {
	ID           int64   `json:"id"`
	DataQuality  *string `json:"data_quality"`
	Title        *string `json:"title"`
	ReleasedYear *int    `json:"released_year"`
}

func (Master) pageItem() {}

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

func (MasterRelease) pageItem() {}

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

func (Release) pageItem() {}

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
	Quantity     *int     `json:"qty"`
	Descriptions []string `json:"descriptions"`
}

type ReleaseVideo struct {
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	Description *string `json:"description"`
}

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
