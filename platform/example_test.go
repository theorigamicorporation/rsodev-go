package platform_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/theorigamicorporation/rsodev-go/platform"
)

// Sign in with a service account and create a queue fed by a topic.
func Example() {
	c, err := platform.New(
		platform.WithTenant(os.Getenv("RSO_TENANT")),
		platform.WithServiceAccount(os.Getenv("RSO_SERVICE_ACCOUNT_NAME"), os.Getenv("RSO_SERVICE_ACCOUNT_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	if _, err := c.CreateQueue(ctx, "orders"); err != nil && !platform.IsConflict(err) {
		log.Fatal(err)
	}
	if err := c.WaitQueueReady(ctx, "orders", platform.WaitOptions{}); err != nil {
		log.Fatal(err)
	}
	if _, err := c.CreateTopic(ctx, "events"); err != nil && !platform.IsConflict(err) {
		log.Fatal(err)
	}
	if _, err := c.SubscribeQueue(ctx, "events", "orders", "order.*"); err != nil {
		log.Fatal(err)
	}

	routed, err := c.PublishToTopic(ctx, "events", "order.created", platform.Message{JSON: map[string]any{"id": 7}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("routed:", routed)
}

// Anything without a typed operation is one GraphQL call away.
func ExampleClient_Do() {
	c, err := platform.New(platform.WithTenant("my-tenant"), platform.WithAccessToken(os.Getenv("RSO_ACCESS_TOKEN")))
	if err != nil {
		log.Fatal(err)
	}
	var out struct {
		Quota []struct {
			Name  string  `json:"name"`
			Used  float64 `json:"used"`
			Limit float64 `json:"limit"`
		} `json:"quota"`
	}
	if err := c.Do(context.Background(), `query { quota { name used limit } }`, nil, &out); err != nil {
		log.Fatal(err)
	}
	for _, row := range out.Quota {
		fmt.Printf("%s: %v of %v\n", row.Name, row.Used, row.Limit)
	}
}

// Deploy a container as an app and wait for its address.
func ExampleClient_CreateApp() {
	c, err := platform.New(
		platform.WithTenant(os.Getenv("RSO_TENANT")),
		platform.WithServiceAccount(os.Getenv("RSO_SERVICE_ACCOUNT_NAME"), os.Getenv("RSO_SERVICE_ACCOUNT_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	_, err = c.CreateApp(ctx, platform.AppInput{
		Name: "web",
		Template: platform.AppTemplate{Spec: platform.AppSpec{Containers: []platform.AppContainer{{
			Image: "registry.example.com/web:1.0.0",
			Ports: []platform.ContainerPort{{ContainerPort: 8080}},
		}}}},
	})
	if err != nil {
		log.Fatal(err)
	}
	app, err := c.WaitAppReady(ctx, "web", platform.WaitOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(*app.Status.URL)
}

// Sign a person in on a device without a browser, then keep the session.
func ExampleOAuth_StartDevice() {
	ctx := context.Background()
	oauth := platform.OAuth{}

	code, err := oauth.StartDevice(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Open", code.VerificationURIComplete, "and confirm the code", code.UserCode)
	session, err := oauth.WaitDevice(ctx, code)
	if err != nil {
		log.Fatal(err)
	}

	// No tenant yet: ask who signed in and which tenants they belong to.
	c, err := platform.New(platform.WithTokenSource(&platform.RefreshingSource{OAuth: oauth, Set: session}))
	if err != nil {
		log.Fatal(err)
	}
	me, err := c.Me(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, tenant := range me.Tenants {
		fmt.Println(tenant.ID, tenant.Name)
	}
}

// List operations that page have a ...Page method; All collects every page.
func ExampleAll() {
	c, err := platform.New(platform.WithTenant("my-tenant"), platform.WithAccessToken(os.Getenv("RSO_ACCESS_TOKEN")))
	if err != nil {
		log.Fatal(err)
	}
	routes, err := platform.All(context.Background(), 50, c.IngressRoutesPage)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(routes), "ingress routes")
}

// Errors are sorted into a few kinds, whatever code the API used.
func ExampleKindOf() {
	c, err := platform.New(platform.WithTenant("my-tenant"), platform.WithAccessToken(os.Getenv("RSO_ACCESS_TOKEN")))
	if err != nil {
		log.Fatal(err)
	}
	_, err = c.CreateBucket(context.Background(), "assets")
	switch platform.KindOf(err) {
	case platform.KindConflict:
		fmt.Println("the bucket already exists")
	case platform.KindQuota:
		fmt.Println("the tenant's quota does not allow another bucket")
	case platform.KindForbidden:
		fmt.Println("this identity may not create buckets")
	}
}
