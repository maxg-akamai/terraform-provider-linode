package linode

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/linode/linodego"
)

func resourceLinodeToken() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceLinodeTokenCreate,
		ReadContext:   resourceLinodeTokenRead,
		UpdateContext: resourceLinodeTokenUpdate,
		DeleteContext: resourceLinodeTokenDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"label": {
				Type:        schema.TypeString,
				Description: "The label of the Linode Token.",
				Optional:    true,
			},
			"scopes": {
				Type: schema.TypeString,
				Description: "The scopes this token was created with. These define what parts of the Account the " +
					"token can be used to access. Many command-line tools, such as the Linode CLI, require tokens with " +
					"access to *. Tokens with more restrictive scopes are generally more secure.",
				Required: true,
				ForceNew: true,
			},
			"expiry": {
				Type: schema.TypeString,
				Description: "When this token will expire. Personal Access Tokens cannot be renewed, so after " +
					"this time the token will be completely unusable and a new token will need to be generated. Tokens " +
					"may be created with 'null' as their expiry and will never expire unless revoked.",
				Optional:         true,
				ValidateFunc:     validDateTime,
				ForceNew:         true,
				DiffSuppressFunc: equivalentDate,
			},
			"created": {
				Type:        schema.TypeString,
				Description: "The date and time this token was created.",
				Computed:    true,
			},
			"token": {
				Type:        schema.TypeString,
				Sensitive:   true,
				Description: "The token used to access the API.",
				Computed:    true,
			},
		},
	}
}

func equivalentDate(k, old, new string, d *schema.ResourceData) bool {
	if dtOld, err := time.Parse("2006-01-02T15:04:05", old); err != nil {
		log.Printf("[WARN] could not parse date %s: %s", old, err)
		return false
	} else if dtNew, err := time.Parse("2006-01-02T15:04:05", new); err != nil {
		log.Printf("[WARN] could not parse date %s: %s", new, err)
		return false
	} else {
		return dtOld.Equal(dtNew)
	}
}

func validDateTime(i interface{}, k string) (s []string, es []error) {
	v, ok := i.(string)
	if !ok {
		es = append(es, fmt.Errorf("expected type of %s to be string", k))
		return
	}
	if _, err := time.Parse("2006-01-02T15:04:05Z", v); err != nil {
		es = append(es, fmt.Errorf("expected %s to be a datetime, got %s", k, v))
	}

	return
}

func resourceLinodeTokenRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*ProviderMeta).Client
	id, err := strconv.ParseInt(d.Id(), 10, 64)
	if err != nil {
		return diag.Errorf("Error parsing Linode Token ID %s as int: %s", d.Id(), err)
	}

	token, err := client.GetToken(ctx, int(id))
	if err != nil {
		if lerr, ok := err.(*linodego.Error); ok && lerr.Code == 404 {
			log.Printf("[WARN] removing Linode Token ID %q from state because it no longer exists", d.Id())
			d.SetId("")
			return nil
		}
		return diag.Errorf("Error finding the specified Linode Token: %s", err)
	}

	d.Set("label", token.Label)
	d.Set("scopes", token.Scopes)
	d.Set("created", token.Created.Format(time.RFC3339))
	d.Set("expiry", token.Expiry.Format(time.RFC3339))

	return nil
}

func resourceLinodeTokenCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*ProviderMeta).Client

	createOpts := linodego.TokenCreateOptions{
		Label:  d.Get("label").(string),
		Scopes: d.Get("scopes").(string),
	}

	if expiryRaw, ok := d.GetOk("expiry"); ok {
		if expiry, ok := expiryRaw.(string); !ok {
			return diag.Errorf("expected expiry to be a string, got %s", expiryRaw)
		} else if dt, err := time.Parse("2006-01-02T15:04:05Z", expiry); err != nil {
			return diag.Errorf("expected expiry to be a datetime, got %s", expiry)
		} else {
			createOpts.Expiry = &dt
		}
	}

	token, err := client.CreateToken(ctx, createOpts)
	if err != nil {
		return diag.Errorf("Error creating a Linode Token: %s", err)
	}
	d.SetId(fmt.Sprintf("%d", token.ID))
	d.Set("token", token.Token)

	return resourceLinodeTokenRead(ctx, d, meta)
}

func resourceLinodeTokenUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*ProviderMeta).Client

	id, err := strconv.ParseInt(d.Id(), 10, 64)
	if err != nil {
		return diag.Errorf("Error parsing Linode Token id %s as int: %s", d.Id(), err)
	}

	token, err := client.GetToken(ctx, int(id))
	if err != nil {
		return diag.Errorf("Error fetching data about the current linode: %s", err)
	}

	updateOpts := token.GetUpdateOptions()
	if d.HasChange("label") {
		updateOpts.Label = d.Get("label").(string)

		if token, err = client.UpdateToken(ctx, token.ID, updateOpts); err != nil {
			return diag.FromErr(err)
		}
		d.Set("label", token.Label)
	}

	return resourceLinodeTokenRead(ctx, d, meta)
}

func resourceLinodeTokenDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*ProviderMeta).Client
	id, err := strconv.ParseInt(d.Id(), 10, 64)
	if err != nil {
		return diag.Errorf("Error parsing Linode Token id %s as int", d.Id())
	}
	err = client.DeleteToken(ctx, int(id))
	if err != nil {
		return diag.Errorf("Error deleting Linode Token %d: %s", id, err)
	}
	// a settling cooldown to avoid expired tokens from being returned in listings
	time.Sleep(3 * time.Second)
	return nil
}
