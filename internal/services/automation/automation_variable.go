package automation

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/automation/mgmt/2018-06-30-preview/automation"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/automation/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func ParseAzureAutomationVariableValue(resource string, input *string) (interface{}, error) {
	if input == nil {
		if resource != "azurerm_automation_variable_null" {
			return nil, fmt.Errorf("Expected value \"nil\" to be %q, actual type is \"azurerm_automation_variable_null\"", resource)
		}
		return nil, nil
	}

	var value interface{}
	var err error
	actualResource := "Unknown"
	datePattern := regexp.MustCompile(`"\\/Date\((-?[0-9]+)\)\\/"`)
	matches := datePattern.FindStringSubmatch(*input)

	if len(matches) == 2 && matches[0] == *input {
		if ticks, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			value = time.Unix(ticks/1000, ticks%1000*1000000).In(time.UTC)
			actualResource = "azurerm_automation_variable_datetime"
		}
	} else if value, err = strconv.Unquote(*input); err == nil {
		actualResource = "azurerm_automation_variable_string"
	} else if value, err = strconv.ParseInt(*input, 10, 32); err == nil {
		value = int32(value.(int64))
		actualResource = "azurerm_automation_variable_int"
	} else if value, err = strconv.ParseBool(*input); err == nil {
		actualResource = "azurerm_automation_variable_bool"
	}

	if actualResource != resource {
		return nil, fmt.Errorf("Expected value %q to be %q, actual type is %q", *input, resource, actualResource)
	}
	return value, nil
}

func resourceAutomationVariableCommonSchema(attType pluginsdk.ValueType, validateFunc pluginsdk.SchemaValidateFunc) map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"resource_group_name": azure.SchemaResourceGroupName(),

		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"automation_account_name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validate.RunbookName(),
		},

		"description": {
			Type:     pluginsdk.TypeString,
			Optional: true,
		},

		"encrypted": {
			Type:     pluginsdk.TypeBool,
			Optional: true,
			Default:  false,
		},

		"value": {
			Type:         attType,
			Optional:     true,
			ValidateFunc: validateFunc,
		},
	}
}

func datasourceAutomationVariableCommonSchema(attType pluginsdk.ValueType) map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"resource_group_name": azure.SchemaResourceGroupName(),

		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"automation_account_name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ValidateFunc: validate.RunbookName(),
		},

		"description": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"encrypted": {
			Type:     pluginsdk.TypeBool,
			Computed: true,
		},

		"value": {
			Type:     attType,
			Computed: true,
		},
	}
}

func resourceAutomationVariableCreateUpdate(d *pluginsdk.ResourceData, meta interface{}, varType string) error {
	client := meta.(*clients.Client).Automation.VariableClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	resourceGroup := d.Get("resource_group_name").(string)
	accountName := d.Get("automation_account_name").(string)
	varTypeLower := strings.ToLower(varType)

	if d.IsNewResource() {
		resp, err := client.Get(ctx, resourceGroup, accountName, name)
		if err != nil {
			if !utils.ResponseWasNotFound(resp.Response) {
				return fmt.Errorf("Error checking for present of existing Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
			}
		}

		if !utils.ResponseWasNotFound(resp.Response) {
			return tf.ImportAsExistsError(fmt.Sprintf("azurerm_automation_variable_%s", varTypeLower), *resp.ID)
		}
	}

	description := d.Get("description").(string)
	encrypted := d.Get("encrypted").(bool)
	value := ""

	switch varTypeLower {
	case "datetime":
		vTime, parseErr := time.Parse(time.RFC3339, d.Get("value").(string))
		if parseErr != nil {
			return fmt.Errorf("Error invalid time format: %+v", parseErr)
		}
		value = fmt.Sprintf("\"\\/Date(%d)\\/\"", vTime.UnixNano()/1000000)
	case "bool":
		value = strconv.FormatBool(d.Get("value").(bool))
	case "int":
		value = strconv.Itoa(d.Get("value").(int))
	case "string":
		value = strconv.Quote(d.Get("value").(string))
	}

	parameters := automation.VariableCreateOrUpdateParameters{
		Name: utils.String(name),
		VariableCreateOrUpdateProperties: &automation.VariableCreateOrUpdateProperties{
			Description: utils.String(description),
			IsEncrypted: utils.Bool(encrypted),
		},
	}

	if varTypeLower != "null" {
		parameters.VariableCreateOrUpdateProperties.Value = utils.String(value)
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroup, accountName, name, parameters); err != nil {
		return fmt.Errorf("Error creating Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
	}

	resp, err := client.Get(ctx, resourceGroup, accountName, name)
	if err != nil {
		return fmt.Errorf("Error retrieving Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
	}
	if resp.ID == nil {
		return fmt.Errorf("Cannot read Automation %s Variable %q (Automation Account Name %q / Resource Group %q) ID", varType, name, accountName, resourceGroup)
	}
	d.SetId(*resp.ID)

	return resourceAutomationVariableRead(d, meta, varType)
}

func resourceAutomationVariableRead(d *pluginsdk.ResourceData, meta interface{}, varType string) error {
	client := meta.(*clients.Client).Automation.VariableClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	accountName := id.Path["automationAccounts"]
	name := id.Path["variables"]
	varTypeLower := strings.ToLower(varType)

	resp, err := client.Get(ctx, resourceGroup, accountName, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Automation %s Variable %q does not exist - removing from state", varType, d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", resourceGroup)
	d.Set("automation_account_name", accountName)
	if properties := resp.VariableProperties; properties != nil {
		d.Set("description", properties.Description)
		d.Set("encrypted", properties.IsEncrypted)
		if !d.Get("encrypted").(bool) {
			value, err := ParseAzureAutomationVariableValue(fmt.Sprintf("azurerm_automation_variable_%s", varTypeLower), properties.Value)
			if err != nil {
				return err
			}

			if varTypeLower == "datetime" {
				d.Set("value", value.(time.Time).Format("2006-01-02T15:04:05.999Z"))
			} else if varTypeLower != "null" {
				d.Set("value", value)
			}
		}
	}

	return nil
}

func dataSourceAutomationVariableRead(d *pluginsdk.ResourceData, meta interface{}, varType string) error {
	client := meta.(*clients.Client).Automation.VariableClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	resourceGroup := d.Get("resource_group_name").(string)
	accountName := d.Get("automation_account_name").(string)
	name := d.Get("name").(string)
	varTypeLower := strings.ToLower(varType)

	resp, err := client.Get(ctx, resourceGroup, accountName, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Automation %s Variable %q does not exist - removing from state", varType, d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
	}

	d.SetId(*resp.ID)

	d.Set("name", resp.Name)
	d.Set("resource_group_name", resourceGroup)
	d.Set("automation_account_name", accountName)
	if properties := resp.VariableProperties; properties != nil {
		d.Set("description", properties.Description)
		d.Set("encrypted", properties.IsEncrypted)
		if !d.Get("encrypted").(bool) {
			value, err := ParseAzureAutomationVariableValue(fmt.Sprintf("azurerm_automation_variable_%s", varTypeLower), properties.Value)
			if err != nil {
				return err
			}

			if varTypeLower == "datetime" {
				d.Set("value", value.(time.Time).Format("2006-01-02T15:04:05.999Z"))
			} else if varTypeLower != "null" {
				d.Set("value", value)
			}
		}
	}

	return nil
}

func resourceAutomationVariableDelete(d *pluginsdk.ResourceData, meta interface{}, varType string) error {
	client := meta.(*clients.Client).Automation.VariableClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	accountName := id.Path["automationAccounts"]
	name := id.Path["variables"]

	if _, err := client.Delete(ctx, resourceGroup, accountName, name); err != nil {
		return fmt.Errorf("Error deleting Automation %s Variable %q (Automation Account Name %q / Resource Group %q): %+v", varType, name, accountName, resourceGroup, err)
	}

	return nil
}
