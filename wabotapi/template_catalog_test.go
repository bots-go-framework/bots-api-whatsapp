package wabotapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTemplates_requestShape pins the GET wire contract: method, path, and
// response decoding including all documented fields.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
func TestListTemplates_requestShape(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "901250373855085",
					"name": "order_confirmation",
					"language": "en_US",
					"status": "APPROVED",
					"category": "UTILITY",
					"components": [
						{"type": "HEADER", "format": "TEXT", "text": "Order Confirmed"},
						{"type": "BODY", "text": "Hi {{1}}, your order {{2}} has been confirmed."},
						{"type": "FOOTER", "text": "Lucky Shrub"}
					],
					"quality_score": {"score": "GREEN"},
					"created_timestamp": 1690000000,
					"last_updated_time": 1690000001
				},
				{
					"id": "901250373855086",
					"name": "seasonal_promo",
					"language": "en_US",
					"status": "PAUSED",
					"category": "MARKETING",
					"components": [
						{"type": "BODY", "text": "Check out our sale!"}
					]
				}
			],
			"paging": {
				"cursors": {
					"before": "before_cursor_value",
					"after": "after_cursor_value"
				},
				"next": "https://graph.facebook.com/v25.0/WABA123/message_templates?after=after_cursor_value"
			}
		}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).ListTemplates(context.Background(), "WABA123", 0, "")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Contains(t, gotPath, "WABA123/message_templates")

	require.Len(t, resp.Data, 2)

	t1 := resp.Data[0]
	assert.Equal(t, "901250373855085", t1.ID)
	assert.Equal(t, "order_confirmation", t1.Name)
	assert.Equal(t, "en_US", t1.Language)
	assert.Equal(t, TemplateStatusApproved, t1.Status)
	assert.Equal(t, TemplateCategoryUtility, t1.Category)
	require.Len(t, t1.Components, 3)
	assert.Equal(t, CatalogComponentTypeHeader, t1.Components[0].Type)
	assert.Equal(t, CatalogComponentFormatText, t1.Components[0].Format)
	assert.Equal(t, "Order Confirmed", t1.Components[0].Text)
	require.NotNil(t, t1.QualityScore)
	assert.Equal(t, "GREEN", t1.QualityScore.Score)
	assert.Equal(t, int64(1690000000), t1.CreatedTimestamp)
	assert.Equal(t, int64(1690000001), t1.LastUpdatedTime)

	t2 := resp.Data[1]
	assert.Equal(t, TemplateStatusPaused, t2.Status)
	assert.Equal(t, TemplateCategoryMarketing, t2.Category)
	assert.Nil(t, t2.QualityScore, "quality_score is absent from the fixture and must not be synthesised")

	require.NotNil(t, resp.Paging)
	require.NotNil(t, resp.Paging.Cursors)
	assert.Equal(t, "after_cursor_value", resp.Paging.Cursors.After)
	assert.NotEmpty(t, resp.Paging.Next)
}

// TestListTemplates_withLimitAndAfterCursor pins that limit and after cursor
// are appended as query parameters when provided.
func TestListTemplates_withLimitAndAfterCursor(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).ListTemplates(context.Background(), "WABA123", 25, "after_cursor_value")
	require.NoError(t, err)
	assert.Contains(t, gotPath, "limit=25")
	assert.Contains(t, gotPath, "after=after_cursor_value")
}

// TestListTemplates_withLimitOnly pins that limit alone produces a correct path.
func TestListTemplates_withLimitOnly(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).ListTemplates(context.Background(), "WABA123", 10, "")
	require.NoError(t, err)
	assert.Contains(t, gotPath, "limit=10")
	assert.NotContains(t, gotPath, "after=")
}

// TestListTemplates_emptyWABAID pins that a blank wabaID fails before any request.
func TestListTemplates_emptyWABAID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).ListTemplates(context.Background(), "", 0, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wabaID")
}

// TestListTemplates_emptyDataPage pins that an empty data array is decoded without error.
func TestListTemplates_emptyDataPage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data": [], "paging": {}}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).ListTemplates(context.Background(), "WABA123", 0, "")
	require.NoError(t, err)
	assert.Empty(t, resp.Data)
}

// TestListTemplates_undecodableResponse pins the decode-failure branch.
func TestListTemplates_undecodableResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["not", "an", "object"]`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).ListTemplates(context.Background(), "WABA123", 0, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}
