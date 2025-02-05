package storage

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/storagesync/mgmt/2020-03-01/storagesync"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/location"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/storage/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/storage/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceStorageSync() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceStorageSyncCreate,
		Read:   resourceStorageSyncRead,
		Update: resourceStorageSyncUpdate,
		Delete: resourceStorageSyncDelete,

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.StorageSyncServiceID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.StorageSyncName,
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"location": azure.SchemaLocation(),

			"incoming_traffic_policy": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Default:  string(storagesync.AllowAllTraffic),
				ValidateFunc: validation.StringInSlice([]string{
					string(storagesync.AllowAllTraffic),
					string(storagesync.AllowVirtualNetworksOnly),
				}, false),
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceStorageSyncCreate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Storage.SyncServiceClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	resourceGroupName := d.Get("resource_group_name").(string)

	existing, err := client.Get(ctx, resourceGroupName, name)
	if err != nil {
		if !utils.ResponseWasNotFound(existing.Response) {
			return fmt.Errorf("Error checking for present of existing Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", name, resourceGroupName, err)
		}
	}
	if existing.ID != nil && *existing.ID != "" {
		return tf.ImportAsExistsError("azurerm_storage_sync", *existing.ID)
	}

	parameters := storagesync.ServiceCreateParameters{
		Location: utils.String(location.Normalize(d.Get("location").(string))),
		ServiceCreateParametersProperties: &storagesync.ServiceCreateParametersProperties{
			IncomingTrafficPolicy: storagesync.IncomingTrafficPolicy(d.Get("incoming_traffic_policy").(string)),
		},
		Tags: tags.Expand(d.Get("tags").(map[string]interface{})),
	}

	future, err := client.Create(ctx, resourceGroupName, name, parameters)
	if err != nil {
		return fmt.Errorf("creating Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", name, resourceGroupName, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for creation of Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", name, resourceGroupName, err)
	}

	resp, err := client.Get(ctx, resourceGroupName, name)
	if err != nil {
		return fmt.Errorf("retrieving Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", name, resourceGroupName, err)
	}
	if resp.ID == nil || *resp.ID == "" {
		return fmt.Errorf("storage Sync(Storage Sync Name %q / Resource Group %q) ID is empty or nil", name, resourceGroupName)
	}
	d.SetId(*resp.ID)

	return resourceStorageSyncRead(d, meta)
}

func resourceStorageSyncRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Storage.SyncServiceClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.StorageSyncServiceID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.Name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Storage Sync %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("reading Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", id.Name, id.ResourceGroup, err)
	}
	d.Set("name", resp.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	d.Set("location", location.NormalizeNilable(resp.Location))
	if props := resp.ServiceProperties; props != nil {
		d.Set("incoming_traffic_policy", props.IncomingTrafficPolicy)
	}
	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceStorageSyncUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Storage.SyncServiceClient
	ctx, cancel := timeouts.ForUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.StorageSyncServiceID(d.Id())
	if err != nil {
		return err
	}

	update := storagesync.ServiceUpdateParameters{}

	if d.HasChange("tags") {
		update.Tags = tags.Expand(d.Get("tags").(map[string]interface{}))
	}

	if d.HasChange("incoming_traffic_policy") {
		update.ServiceUpdateProperties = &storagesync.ServiceUpdateProperties{
			IncomingTrafficPolicy: storagesync.IncomingTrafficPolicy(d.Get("incoming_traffic_policy").(string)),
		}
	}

	future, err := client.Update(ctx, id.ResourceGroup, id.Name, &update)
	if err != nil {
		return fmt.Errorf("updating Storage Sync %q (Resource Group %q): %+v", id.Name, id.ResourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for update of Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", id.Name, id.ResourceGroup, err)
	}

	return resourceStorageSyncRead(d, meta)
}

func resourceStorageSyncDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Storage.SyncServiceClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.StorageSyncServiceID(d.Id())
	if err != nil {
		return err
	}

	future, err := client.Delete(ctx, id.ResourceGroup, id.Name)
	if err != nil {
		return fmt.Errorf("deleting Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", id.Name, id.ResourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for deletion of Storage Sync(Storage Sync Name %q / Resource Group %q): %+v", id.Name, id.ResourceGroup, err)
	}

	return nil
}
