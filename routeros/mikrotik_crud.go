package routeros

import (
	"context"
	"fmt"
)

// resource path is '/interface/vlan' etc.
// resource query is '/id' or '?.id=*39'.
var (
	errEmptyId   = fmt.Errorf("the resource id not defined")
	errEmptyItem = fmt.Errorf("the item is null")
	errEmptyPath = fmt.Errorf("the resource path not defined")
)

// REST command suffixes used by context-selected create operations.
var restCommandSuffix = map[crudMethod]string{
	crudCreate: "/add", crudRead: "/print", crudUpdate: "/set", crudDelete: "/remove",
	crudPost: "/set", crudImport: "/import", crudSign: "/sign", crudSignViaScep: "/add-scep",
	crudRemove: "/remove", crudRevoke: "/issued-revoke", crudMove: "/move", crudStart: "/start",
	crudStop: "/stop", crudGenerateKey: "/generate-key", crudUnset: "/unset", crudPrintConfig: "/print",
}

// https://help.mikrotik.com/docs/display/ROS/REST+API

func CreateItem(ctx context.Context, item MikrotikItem, resourcePath string, c Client) (MikrotikItem, error) {
	if item == nil {
		return nil, errEmptyItem
	}
	if resourcePath == "" {
		return nil, errEmptyPath
	}

	var crud = crudCreate

	if cm := ctxGetCrudMethod(ctx); cm != crudUnknown {
		crud = cm

		// REST command methods use explicit console command paths.
		resourcePath += restCommandSuffix[crud]
	}

	res := MikrotikItem{}
	err := c.SendRequest(crud, &URL{Path: resourcePath}, item, &res)

	return res, err
}

func ReadItems(id *ItemId, resourcePath string, c Client) (*[]MikrotikItem, error) {
	// id can be empty.

	if resourcePath == "" {
		return nil, errEmptyPath
	}

	url := &URL{Path: resourcePath}

	// If the 'id' is nil, then this is a Datasource reading (resource Path only).
	if id != nil {
		// REST: prevent 404 'Not Found' error by direct resource request (/interface/vlan/*39).
		// Error occurs when a resource has been deleted outside terraform control.
		// But in the case below we have an empty [] or non-empty array [{...}].

		// /interface/vlan?.id=*39
		url.Query = []string{"?" + id.Type.String() + "=" + id.Value}
	}

	var res []MikrotikItem
	err := c.SendRequest(crudRead, url, nil, &res)

	return &res, err
}

func ReadItemsFiltered(filter []string, resourcePath string, c Client) (*[]MikrotikItem, error) {
	if resourcePath == "" {
		return nil, errEmptyPath
	}

	// Filter format: name=value
	// REST query: name=value; name=value

	url := &URL{Path: resourcePath, Query: filter}

	var res []MikrotikItem
	err := c.SendRequest(crudRead, url, nil, &res)

	return &res, err
}

func UpdateItem(id *ItemId, resourcePath string, item MikrotikItem, c Client) (MikrotikItem, error) {
	if id.Value == "" {
		return nil, errEmptyId
	}
	if resourcePath == "" {
		return nil, errEmptyPath
	}

	// /interface/vlan/*39
	resourcePath += "/" + id.Value

	res := MikrotikItem{}
	err := c.SendRequest(crudUpdate, &URL{Path: resourcePath}, item, &res)

	return res, err
}

func DeleteItem(id *ItemId, resourcePath string, c Client) error {
	if id.Value == "" {
		return errEmptyId
	}
	if resourcePath == "" {
		return errEmptyPath
	}

	url := &URL{Path: resourcePath}

	// This method is used to delete the record with a specified ID from the menu encoded in the URL.
	// If the deletion has been succeeded, the server responds with an empty response.
	// For example, call to delete the record twice, on second call router will return 404 error.

	// /interface/vlan/*39
	url.Path += "/" + id.Value

	return c.SendRequest(crudDelete, url, nil, &MikrotikItem{})
}
