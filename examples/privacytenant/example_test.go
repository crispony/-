// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"entgo.io/ent/examples/privacytenant/ent"
	"entgo.io/ent/examples/privacytenant/ent/privacy"
	_ "entgo.io/ent/examples/privacytenant/ent/runtime"
	"entgo.io/ent/examples/privacytenant/viewer"

	_ "github.com/mattn/go-sqlite3"
)

func Example_PrivacyTenant() {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	// Run the auto migration tool.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	if err := Do(ctx, client); err != nil {
		log.Fatal(err)
	}
	// Output:
	// Tenant(id=1, name=GitHub)
	// Tenant(id=2, name=GitLab)
	// User(id=1, name=a8m, foods=[])
	// User(id=2, name=nati, foods=[])
	// Group(id=1, name=entgo.io)
	// Group(id=1, name=entgo)
}

func Do(ctx context.Context, client *ent.Client) error {
	// Expect operation to fail, because viewer-context
	// is missing (first mutation rule check).
	if err := client.Tenant.Create().Exec(ctx); !errors.Is(err, privacy.Deny) {
		return fmt.Errorf("expect operation to fail, but got %w", err)
	}
	// Deny tenant creation if the viewer is not admin.
	viewCtx := viewer.NewContext(ctx, viewer.UserViewer{Role: viewer.View})
	if err := client.Tenant.Create().Exec(viewCtx); !errors.Is(err, privacy.Deny) {
		return fmt.Errorf("expect operation to fail, but got %w", err)
	}
	// Apply the same operation with "Admin" role, expect it to pass.
	adminCtx := viewer.NewContext(ctx, viewer.UserViewer{Role: viewer.Admin})
	hub, err := client.Tenant.Create().SetName("GitHub").Save(adminCtx)
	if err != nil {
		return fmt.Errorf("expect operation to pass, but got %w", err)
	}
	fmt.Println(hub)
	lab, err := client.Tenant.Create().SetName("GitLab").Save(adminCtx)
	if err != nil {
		return fmt.Errorf("expect operation to pass, but got %w", err)
	}
	fmt.Println(lab)

	// Create 2 users connected to the 2 tenants we created above
	hubUser := client.User.Create().SetName("a8m").SetTenant(hub).SaveX(adminCtx)
	labUser := client.User.Create().SetName("nati").SetTenant(lab).SaveX(adminCtx)

	hubView := viewer.NewContext(ctx, viewer.UserViewer{T: hub})
	out := client.User.Query().OnlyX(hubView)
	// Expect that "GitHub" tenant to read only its users (i.e. a8m).
	if out.ID != hubUser.ID {
		return fmt.Errorf("expect result for user query, got %v", out)
	}
	fmt.Println(out)

	labView := viewer.NewContext(ctx, viewer.UserViewer{T: lab})
	out = client.User.Query().OnlyX(labView)
	// Expect that "GitLab" tenant to read only its users (i.e. nati).
	if out.ID != labUser.ID {
		return fmt.Errorf("expect result for user query, got %v", out)
	}
	fmt.Println(out)

	// Expect operation to fail, because the DenyMismatchedTenants rule makes sure
	// the group and the users are connected to the same tenant.
	err = client.Group.Create().SetName("entgo.io").SetTenant(hub).AddUsers(labUser).Exec(adminCtx)
	if !errors.Is(err, privacy.Deny) {
		return fmt.Errorf("expect operation to fail, since user (nati) is not connected to the same tenant")
	}
	err = client.Group.Create().SetName("entgo.io").SetTenant(hub).AddUsers(labUser, hubUser).Exec(adminCtx)
	if !errors.Is(err, privacy.Deny) {
		return fmt.Errorf("expect operation to fail, since some users (nati) are not connected to the same tenant")
	}
	entgo, err := client.Group.Create().SetName("entgo.io").SetTenant(hub).AddUsers(hubUser).Save(adminCtx)
	if err != nil {
		return fmt.Errorf("expect operation to pass, but got %w", err)
	}
	fmt.Println(entgo)

	// Expect operation to fail, because the FilterTenantRule rule makes sure
	// that tenants can update and delete only their groups.
	err = entgo.Update().SetName("fail.go").Exec(labView)
	if !ent.IsNotFound(err) {
		return fmt.Errorf("expect operation to fail, since the group (entgo) is managed by a different tenant (hub), but got %w", err)
	}
	entgo, err = entgo.Update().SetName("entgo").Save(hubView)
	if err != nil {
		return fmt.Errorf("expect operation to pass, but got %w", err)
	}
	fmt.Println(entgo)

	return nil
}
