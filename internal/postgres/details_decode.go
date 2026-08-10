package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	decodeArtistDetailError  = "decode artist detail"
	decodeLabelDetailError   = "decode label detail"
	decodeMasterDetailError  = "decode master detail"
	decodeReleaseDetailError = "decode release detail"
)

type artistDetailPayload struct {
	members    []byte
	groups     []byte
	aliases    []byte
	variations []byte
	urls       []byte
}

func decodeArtistDetail(detail *catalog.ArtistDetail, payload artistDetailPayload) error {
	err := errors.Join(
		json.Unmarshal(payload.members, &detail.Members),
		json.Unmarshal(payload.groups, &detail.Groups),
		json.Unmarshal(payload.aliases, &detail.Aliases),
		json.Unmarshal(payload.variations, &detail.NameVariations),
		json.Unmarshal(payload.urls, &detail.URLs),
	)
	return decodeError(decodeArtistDetailError, err)
}

func artistDetailResult(detail catalog.ArtistDetail, payload artistDetailPayload) (catalog.ArtistDetail, error) {
	if err := decodeArtistDetail(&detail, payload); err != nil {
		return catalog.ArtistDetail{}, err
	}
	return detail, nil
}

type labelDetailPayload struct {
	parent    []byte
	sublabels []byte
	urls      []byte
}

func decodeLabelDetail(detail *catalog.LabelDetail, payload labelDetailPayload) error {
	var parentError error
	if len(payload.parent) > 0 && string(payload.parent) != "null" {
		detail.ParentLabel = &catalog.LabelReference{}
		parentError = json.Unmarshal(payload.parent, detail.ParentLabel)
	}
	err := errors.Join(
		parentError,
		json.Unmarshal(payload.sublabels, &detail.Sublabels),
		json.Unmarshal(payload.urls, &detail.URLs),
	)
	return decodeError(decodeLabelDetailError, err)
}

func labelDetailResult(detail catalog.LabelDetail, payload labelDetailPayload) (catalog.LabelDetail, error) {
	if err := decodeLabelDetail(&detail, payload); err != nil {
		return catalog.LabelDetail{}, err
	}
	return detail, nil
}

type masterDetailPayload struct {
	genres  []byte
	styles  []byte
	artists []byte
	videos  []byte
}

func decodeMasterDetail(detail *catalog.MasterDetail, payload masterDetailPayload) error {
	err := errors.Join(
		json.Unmarshal(payload.genres, &detail.Genres),
		json.Unmarshal(payload.styles, &detail.Styles),
		json.Unmarshal(payload.artists, &detail.Artists),
		json.Unmarshal(payload.videos, &detail.Videos),
	)
	return decodeError(decodeMasterDetailError, err)
}

func masterDetailResult(detail catalog.MasterDetail, payload masterDetailPayload) (catalog.MasterDetail, error) {
	if err := decodeMasterDetail(&detail, payload); err != nil {
		return catalog.MasterDetail{}, err
	}
	return detail, nil
}

type releaseDetailPayload struct {
	artists   []byte
	labels    []byte
	companies []byte
	formats   []byte
	styles    []byte
	genres    []byte
	videos    []byte
}

func decodeReleaseDetail(detail *catalog.ReleaseDetail, payload releaseDetailPayload) error {
	err := errors.Join(
		json.Unmarshal(payload.artists, &detail.Artists),
		json.Unmarshal(payload.labels, &detail.Labels),
		json.Unmarshal(payload.companies, &detail.Companies),
		json.Unmarshal(payload.formats, &detail.Formats),
		json.Unmarshal(payload.styles, &detail.Styles),
		json.Unmarshal(payload.genres, &detail.Genres),
		json.Unmarshal(payload.videos, &detail.Videos),
	)
	return decodeError(decodeReleaseDetailError, err)
}

func releaseDetailResult(detail catalog.ReleaseDetail, payload releaseDetailPayload) (catalog.ReleaseDetail, error) {
	if err := decodeReleaseDetail(&detail, payload); err != nil {
		return catalog.ReleaseDetail{}, err
	}
	return detail, nil
}

func decodeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
