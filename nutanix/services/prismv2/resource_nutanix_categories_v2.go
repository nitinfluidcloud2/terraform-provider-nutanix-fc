package prismv2

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	import1 "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	prismsdk "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/prism"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixCategoriesV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: ResourceNutanixCategoriesV2Create,
		ReadContext:   ResourceNutanixCategoriesV2Read,
		UpdateContext: ResourceNutanixCategoriesV2Update,
		DeleteContext: ResourceNutanixCategoriesV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"key": {
				Type:     schema.TypeString,
				Required: true,
			},
			"value": {
				Type:     schema.TypeString,
				Required: true,
			},
			"type": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.StringInSlice([]string{"USER", "INTERNAL", "SYSTEM"}, false),
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"owner_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"associations": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"category_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"resource_type": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"resource_group": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"count": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"detailed_associations": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"category_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"resource_type": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"resource_group": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"resource_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			// parent_ext_id tracks the parent "key" bucket extId so Delete can
			// clean it up when this resource is the last child. The Nutanix
			// backend implicitly creates a parent bucket on CreateCategory; the
			// GA CRUD API returns only the child leaf's extId, so we discover
			// the parent via the v4.0.a1 listing path (same one the Prism UI
			// uses) and stash it here.
			"parent_ext_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Internal: extId of the parent 'key' bucket. Used by Delete to clean up the bucket when this resource is its last child.",
			},
		},
	}
}

func ResourceNutanixCategoriesV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).PrismAPI

	input := &import1.Category{}

	if key, ok := d.GetOk("key"); ok {
		input.Key = utils.StringPtr(key.(string))
	}
	if val, ok := d.GetOk("value"); ok {
		input.Value = utils.StringPtr(val.(string))
	}
	if types, ok := d.GetOk("type"); ok {
		const two, three, four = 2, 3, 4
		subMap := map[string]interface{}{
			"USER":     two,
			"SYSTEM":   three,
			"INTERNAL": four,
		}

		pInt := subMap[types.(string)]
		p := import1.CategoryType(pInt.(int))

		input.Type = &p
	}
	if desc, ok := d.GetOk("description"); ok {
		input.Description = utils.StringPtr(desc.(string))
	}
	if ownerUUID, ok := d.GetOk("owner_uuid"); ok {
		input.OwnerUuid = utils.StringPtr(ownerUUID.(string))
	}

	resp, err := conn.CategoriesAPIInstance.CreateCategory(input)
	if err != nil {
		return diag.Errorf("error while creating category: %v", err)
	}

	getResp := resp.Data.GetValue().(import1.Category)

	d.SetId(utils.StringValue(getResp.ExtId))

	// Best-effort: discover and persist the parent "key" bucket extId so
	// Delete can clean it up when this resource is the last child under that
	// key. The GA Category model does not expose parentExtId; we mirror the
	// Prism UI's pattern and use the v4.0.a1 listing endpoint for discovery.
	// A failure here does NOT fail apply — the worst case is the existing
	// orphan-parent behavior.
	categoryKey := utils.StringValue(getResp.Key)
	parentID, perr := discoverParentBucketExtID(conn, categoryKey)
	if perr != nil {
		log.Printf("[WARN] could not discover parent bucket for category key %q: %v", categoryKey, perr)
	} else if parentID == "" {
		log.Printf("[DEBUG] no parent bucket discovered for category key %q (Delete cleanup will be a no-op)", categoryKey)
	} else {
		log.Printf("[DEBUG] discovered parent bucket %s for category key %q", parentID, categoryKey)
		_ = d.Set("parent_ext_id", parentID)
	}

	return ResourceNutanixCategoriesV2Read(ctx, d, meta)
}

func ResourceNutanixCategoriesV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).PrismAPI

	resp, err := conn.CategoriesAPIInstance.GetCategoryById(utils.StringPtr(d.Id()), nil)
	if err != nil {
		return diag.Errorf("error while fetching category : %v", err)
	}

	getResp := resp.Data.GetValue().(import1.Category)

	if err := d.Set("key", getResp.Key); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("value", getResp.Value); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("type", flattenCategoryType(getResp.Type)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("description", getResp.Description); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("owner_uuid", getResp.OwnerUuid); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("associations", flattenAssociationSummary(getResp.Associations)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("detailed_associations", flattenAssociationDetail(getResp.DetailedAssociations)); err != nil {
		return diag.FromErr(err)
	}

	// Repopulate parent_ext_id if missing (e.g. after `terraform import`).
	// Best-effort: do not fail Read on a discovery error.
	if d.Get("parent_ext_id").(string) == "" {
		if parentID, perr := discoverParentBucketExtID(conn, utils.StringValue(getResp.Key)); perr == nil && parentID != "" {
			_ = d.Set("parent_ext_id", parentID)
		}
	}
	return nil
}

func ResourceNutanixCategoriesV2Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).PrismAPI
	updatedInput := import1.Category{}
	resp, err := conn.CategoriesAPIInstance.GetCategoryById(utils.StringPtr(d.Id()), nil)
	if err != nil {
		return diag.Errorf("error while fetching categories : %v", err)
	}

	updatedInput = resp.Data.GetValue().(import1.Category)

	if d.HasChange("value") {
		updatedInput.Value = utils.StringPtr(d.Get("value").(string))
	}
	if d.HasChange("description") {
		updatedInput.Description = utils.StringPtr(d.Get("description").(string))
	}
	if d.HasChange("type") {
		const two, three, four = 2, 3, 4
		subMap := map[string]interface{}{
			"USER":     two,
			"SYSTEM":   three,
			"INTERNAL": four,
		}

		pInt := subMap[d.Get("type").(string)]
		p := import1.CategoryType(pInt.(int))
		updatedInput.Type = &p
	}
	if d.HasChange("owner_uuid") {
		updatedInput.OwnerUuid = utils.StringPtr(d.Get("owner_uuid").(string))
	}

	_, er := conn.CategoriesAPIInstance.UpdateCategoryById(utils.StringPtr(d.Id()), &updatedInput)
	if er != nil {
		return diag.Errorf("error while updating categories : %v", err)
	}
	log.Println("[DEBUG] Category updated successfully")
	return ResourceNutanixCategoriesV2Read(ctx, d, meta)
}

func ResourceNutanixCategoriesV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).PrismAPI

	// 1. Delete the child leaf (the extId tfstate has been tracking all along).
	resp, err := conn.CategoriesAPIInstance.DeleteCategoryById(utils.StringPtr(d.Id()))
	if err != nil {
		return diag.Errorf("error while deleting category : %v", err)
	}

	if resp == nil {
		log.Println("[DEBUG] Category deleted successfully.")
	}

	// 2. Clean up the parent "key" bucket if this resource was its last child.
	// Without this, the bucket leaks on the cluster after every destroy cycle.
	parentID := d.Get("parent_ext_id").(string)
	if parentID == "" {
		return nil
	}
	empty, err := isParentBucketEmpty(conn, parentID)
	if err != nil {
		log.Printf("[WARN] could not verify parent bucket %s is empty after child delete: %v", parentID, err)
		return nil
	}
	if !empty {
		log.Printf("[DEBUG] parent bucket %s still has sibling categories; not deleting", parentID)
		return nil
	}
	// Parent buckets are a v4.0.a1-only construct on PC 7.5. v4.2 / v4.3
	// DELETE returns 404 for the parent extId; only v4.0.a1 recognizes it.
	// Try v4.0.a1 first; fall back to v4.2 in case the cluster behavior
	// differs across releases.
	for _, ver := range []string{"v4.0.a1", "v4.2"} {
		uri := "/api/prism/" + ver + "/config/categories/" + parentID
		body, derr := callPrismRawStrict(conn, "DELETE", uri)
		if derr == nil {
			log.Printf("[INFO] empty parent category bucket %s deleted via %s", parentID, ver)
			return nil
		}
		log.Printf("[DEBUG] DELETE %s on parent bucket %s failed: %v (body=%s)", ver, parentID, derr, string(body))
	}
	log.Printf("[WARN] could not delete empty parent category bucket %s on any API version", parentID)
	return nil
}

// -----------------------------------------------------------------------------
// Helpers for the orphan-parent-bucket cleanup.
//
// The Nutanix v4 backend stores each "category" as a two-level tree:
//   - a parent "key" bucket (extId K, name=<key>, no value, parentExtId=null)
//   - one or more child "value" leaves under that bucket
//     (extId C, name=<value>, parentExtId=K)
//
// The GA v4.3 Category list/get shape does NOT expose parentExtId or
// childCategories. We mirror what the Prism UI does and use the v4.0.a1
// alpha endpoint for discovery only — the GA SDK Delete path is reused
// for the actual removal so the version-negotiation flow is untouched.
// -----------------------------------------------------------------------------

// rawCategorySummary mirrors the subset of fields we read from
// /api/prism/v4.0.a1/config/categories.
type rawCategorySummary struct {
	ExtId           *string              `json:"extId"`
	Name            *string              `json:"name"`
	ParentExtId     *string              `json:"parentExtId"`
	Type            *string              `json:"type"`
	ChildCategories []rawCategorySummary `json:"childCategories"`
}

type rawCategoryListResponse struct {
	Data []rawCategorySummary `json:"data"`
}

type rawCategoryGetResponse struct {
	Data rawCategorySummary `json:"data"`
}

// discoverParentBucketExtID returns the extId of the parent "key" bucket
// for a category with the given key, by listing the v4.0.a1 alpha endpoint
// with $filter=name eq '<key>' and parentExtId eq null. Empty result →
// empty string and no error.
func discoverParentBucketExtID(conn *prismsdk.Client, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	filter := fmt.Sprintf("name eq '%s' and parentExtId eq null", key)
	q := url.Values{}
	q.Set("$page", "0")
	q.Set("$limit", "1")
	q.Set("$filter", filter)
	uri := "/api/prism/v4.0.a1/config/categories?" + q.Encode()

	body, err := callPrismRaw(conn, "GET", uri)
	if err != nil {
		return "", fmt.Errorf("list categories (alpha): %w", err)
	}
	var parsed rawCategoryListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse alpha list response: %w", err)
	}
	for _, c := range parsed.Data {
		if c.ExtId == nil {
			continue
		}
		if c.ParentExtId != nil && *c.ParentExtId != "" {
			continue
		}
		return *c.ExtId, nil
	}
	return "", nil
}

// isParentBucketEmpty returns true if the parent bucket has zero child
// categories. Implemented against the v4.0.a1 alpha endpoint with
// $expand=childCategories. If the bucket itself is already gone we treat
// that as "empty" so the destroy path is idempotent.
func isParentBucketEmpty(conn *prismsdk.Client, parentExtID string) (bool, error) {
	if parentExtID == "" {
		return false, fmt.Errorf("empty parentExtID")
	}
	uri := "/api/prism/v4.0.a1/config/categories/" + parentExtID + "?$expand=childCategories"
	body, err := callPrismRaw(conn, "GET", uri)
	if err != nil {
		return true, nil
	}
	var parsed rawCategoryGetResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, fmt.Errorf("parse alpha get response: %w", err)
	}
	return len(parsed.Data.ChildCategories) == 0, nil
}

// callPrismRaw issues an HTTP request to the Prism Central host using the
// same credentials wired into the typed SDK, WITHOUT going through the SDK's
// CallApi — which silently rewrites the URI path's version component to the
// SDK's negotiated version (e.g. v4.0.a1 → v4.2 on PC 7.5). The version
// rewrite would defeat our discovery (v4.2 GA does not expose
// parentExtId/childCategories), so we bypass it by talking directly to the
// host with net/http while reusing Host/Port/Username/Password/VerifySSL
// from the already-configured ApiClient.
func callPrismRaw(conn *prismsdk.Client, method, uri string) ([]byte, error) {
	if conn == nil || conn.CategoriesAPIInstance == nil || conn.CategoriesAPIInstance.ApiClient == nil {
		return nil, fmt.Errorf("prism api client is nil")
	}
	ac := conn.CategoriesAPIInstance.ApiClient
	if ac.Host == "" {
		return nil, fmt.Errorf("prism api client host is empty")
	}
	port := ac.Port
	if port == 0 {
		port = 9440
	}
	target := fmt.Sprintf("https://%s:%d%s", ac.Host, port, uri)
	log.Printf("[DEBUG] callPrismRaw %s %s", method, target)

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(ac.Username, ac.Password)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !ac.VerifySSL},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s %s: %w", method, target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	log.Printf("[DEBUG] callPrismRaw response HTTP %d, body bytes=%d", resp.StatusCode, len(body))
	if resp.StatusCode == http.StatusNotFound {
		// Treat 404 as "nothing here" so the caller can decide whether
		// that's the success-path (e.g. parent already gone in destroy).
		return body, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s %s: %s", resp.StatusCode, method, target, string(body))
	}
	return body, nil
}

// callPrismRawStrict is like callPrismRaw but treats EVERY 4xx/5xx
// (including 404) as an error. Used for the parent-delete fallback loop
// where we need to know whether the call actually succeeded so we can
// move on to the next API version.
func callPrismRawStrict(conn *prismsdk.Client, method, uri string) ([]byte, error) {
	if conn == nil || conn.CategoriesAPIInstance == nil || conn.CategoriesAPIInstance.ApiClient == nil {
		return nil, fmt.Errorf("prism api client is nil")
	}
	ac := conn.CategoriesAPIInstance.ApiClient
	if ac.Host == "" {
		return nil, fmt.Errorf("prism api client host is empty")
	}
	port := ac.Port
	if port == 0 {
		port = 9440
	}
	target := fmt.Sprintf("https://%s:%d%s", ac.Host, port, uri)
	log.Printf("[DEBUG] callPrismRawStrict %s %s", method, target)

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(ac.Username, ac.Password)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !ac.VerifySSL},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s %s: %w", method, target, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DEBUG] callPrismRawStrict response HTTP %d, body bytes=%d", resp.StatusCode, len(body))
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}
