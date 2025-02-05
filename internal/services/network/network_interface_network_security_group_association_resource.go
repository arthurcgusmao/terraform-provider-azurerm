package network

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2020-11-01/network"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/azuresdkhacks"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/locks"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceNetworkInterfaceSecurityGroupAssociation() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceNetworkInterfaceSecurityGroupAssociationCreate,
		Read:   resourceNetworkInterfaceSecurityGroupAssociationRead,
		Delete: resourceNetworkInterfaceSecurityGroupAssociationDelete,
		// TODO: replace this with an importer which validates the ID during import
		Importer: pluginsdk.DefaultImporter(),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"network_interface_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"network_security_group_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},
		},
	}
}

func resourceNetworkInterfaceSecurityGroupAssociationCreate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	log.Printf("[INFO] preparing arguments for Network Interface <-> Network Security Group Association creation.")

	networkInterfaceId := d.Get("network_interface_id").(string)
	networkSecurityGroupId := d.Get("network_security_group_id").(string)

	nicId, err := azure.ParseAzureResourceID(networkInterfaceId)
	if err != nil {
		return err
	}

	networkInterfaceName := nicId.Path["networkInterfaces"]
	resourceGroup := nicId.ResourceGroup

	locks.ByName(networkInterfaceName, networkInterfaceResourceName)
	defer locks.UnlockByName(networkInterfaceName, networkInterfaceResourceName)

	nsgId, err := azure.ParseAzureResourceID(networkSecurityGroupId)
	if err != nil {
		return err
	}
	nsgName := nsgId.Path["networkSecurityGroups"]

	locks.ByName(nsgName, networkSecurityGroupResourceName)
	defer locks.UnlockByName(nsgName, networkSecurityGroupResourceName)

	read, err := client.Get(ctx, resourceGroup, networkInterfaceName, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			return fmt.Errorf("Network Interface %q (Resource Group %q) was not found!", networkInterfaceName, resourceGroup)
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	props := read.InterfacePropertiesFormat
	if props == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}

	// first double-check it doesn't exist
	resourceId := fmt.Sprintf("%s|%s", networkInterfaceId, networkSecurityGroupId)
	if props.NetworkSecurityGroup != nil {
		return tf.ImportAsExistsError("azurerm_network_interface_security_group_association", resourceId)
	}

	props.NetworkSecurityGroup = &network.SecurityGroup{
		ID: utils.String(networkSecurityGroupId),
	}

	future, err := client.CreateOrUpdate(ctx, resourceGroup, networkInterfaceName, read)
	if err != nil {
		return fmt.Errorf("Error updating Security Group Association for Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for completion of Security Group Association for NIC %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	d.SetId(resourceId)

	return resourceNetworkInterfaceSecurityGroupAssociationRead(d, meta)
}

func resourceNetworkInterfaceSecurityGroupAssociationRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	splitId := strings.Split(d.Id(), "|")
	if len(splitId) != 2 {
		return fmt.Errorf("Expected ID to be in the format {networkInterfaceId}|{networkSecurityGroupId} but got %q", d.Id())
	}

	nicID, err := azure.ParseAzureResourceID(splitId[0])
	if err != nil {
		return err
	}

	name := nicID.Path["networkInterfaces"]
	resourceGroup := nicID.ResourceGroup

	read, err := client.Get(ctx, resourceGroup, name, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			log.Printf("Network Interface %q (Resource Group %q) was not found - removing from state!", name, resourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	props := read.InterfacePropertiesFormat
	if props == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", name, resourceGroup)
	}

	if props.NetworkSecurityGroup == nil || props.NetworkSecurityGroup.ID == nil {
		log.Printf("Network Interface %q (Resource Group %q) doesn't have a Security Group attached - removing from state!", name, resourceGroup)
		d.SetId("")
		return nil
	}

	d.Set("network_interface_id", read.ID)

	// nil-checked above
	d.Set("network_security_group_id", props.NetworkSecurityGroup.ID)

	return nil
}

func resourceNetworkInterfaceSecurityGroupAssociationDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	splitId := strings.Split(d.Id(), "|")
	if len(splitId) != 2 {
		return fmt.Errorf("Expected ID to be in the format {networkInterfaceId}/{networkSecurityGroup} but got %q", d.Id())
	}

	nicID, err := azure.ParseAzureResourceID(splitId[0])
	if err != nil {
		return err
	}

	name := nicID.Path["networkInterfaces"]
	resourceGroup := nicID.ResourceGroup

	locks.ByName(name, networkInterfaceResourceName)
	defer locks.UnlockByName(name, networkInterfaceResourceName)

	read, err := client.Get(ctx, resourceGroup, name, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			return fmt.Errorf("Network Interface %q (Resource Group %q) was not found!", name, resourceGroup)
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	props := read.InterfacePropertiesFormat
	if props == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", name, resourceGroup)
	}

	props.NetworkSecurityGroup = nil
	read.InterfacePropertiesFormat = props

	future, err := azuresdkhacks.UpdateNetworkInterfaceAllowingRemovalOfNSG(ctx, client, resourceGroup, name, read)
	if err != nil {
		return fmt.Errorf("Error updating Network Interface %q (Resource Group %q): %+v", name, resourceGroup, err)
	}
	if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for update of Network Interface %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	return nil
}
