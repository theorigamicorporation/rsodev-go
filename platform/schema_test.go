package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

type recorded struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// TestDocumentsMatchTheSchema calls every typed operation against a server
// that only records what it is sent, and then checks each recorded document
// and its variables against the schema snapshot in testdata. An operation,
// field, argument or input the platform does not have fails here. The
// snapshot is refreshed with internal/schemasnap.
func TestDocumentsMatchTheSchema(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "schema.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "schema.graphql", Input: string(source)})
	if err != nil {
		t.Fatal(err)
	}

	var calls []recorded
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/graphql" {
			var call recorded
			_ = json.Unmarshal(raw, &call)
			calls = append(calls, call)
		}
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()
	c, err := New(WithTenant("acme"), WithAccessToken("token"), WithEndpoint(server.URL+"/graphql"))
	if err != nil {
		t.Fatal(err)
	}
	everyOperation(context.Background(), c)

	if len(calls) < 150 {
		t.Fatalf("only %d operations were recorded; everyOperation is incomplete", len(calls))
	}
	seen := map[string]bool{}
	for _, call := range calls {
		if seen[call.Query] {
			continue
		}
		seen[call.Query] = true
		document, errs := gqlparser.LoadQuery(schema, call.Query)
		if len(errs) > 0 {
			t.Errorf("%s\n  %v", call.Query, errs)
			continue
		}
		for _, operation := range document.Operations {
			if _, err := validator.VariableValues(schema, operation, call.Variables); err != nil {
				t.Errorf("%s\n  variables %v: %v", call.Query, call.Variables, err)
			}
		}
	}
}

// everyOperation calls each typed operation once. Results and errors do not
// matter here; the requests do.
func everyOperation(ctx context.Context, c *Client) {
	text, yes, n := "x", true, 1
	_, _ = c.Me(ctx)
	_, _ = c.Permissions(ctx)
	_, _ = c.Quota(ctx)
	_, _ = c.CostEstimate(ctx, EstimateInput{Instances: []EstimateItem{{VCPU: 2, RAMGB: 4, DiskGB: 20, Count: 1}}, Apps: []EstimateItem{{VCPU: 1, RAMGB: 1}}, Postgres: []EstimateItem{{VCPU: 1, RAMGB: 2, DiskGB: 10, Count: 2}}})
	_, _ = c.PlatformEvents(ctx)

	app := AppInput{Name: "web", Metadata: &AppMetadata{Labels: []KeyValue{{Key: "k", Value: "v"}}}, Template: AppTemplate{
		Metadata: &AppTemplateMetadata{Annotations: []KeyValue{{Key: "k", Value: "v"}}},
		Spec: AppSpec{
			ImagePullSecrets: []PullSecret{{Name: "pull"}},
			Volumes:          []AppVolume{{Name: "data", Claim: &ClaimVolume{ClaimName: "data"}}, {Name: "conf", Secret: &SecretVolume{SecretName: "conf", Optional: &yes}}},
			Containers: []AppContainer{{
				Image:        "registry.example.com/web:1",
				Env:          []EnvVar{{Name: "A", Value: &text}, {Name: "B", ValueFrom: &EnvSource{SecretKeyRef: &SecretKeyRef{Name: "s", Key: "k", Optional: &yes}}}},
				Ports:        []ContainerPort{{ContainerPort: 8080, Name: &text}},
				Resources:    &Resources{Requests: &ResourceList{CPU: &text, Memory: &text}, Limits: &ResourceList{CPU: &text}},
				VolumeMounts: []VolumeMount{{Name: "data", MountPath: "/data", ReadOnly: &yes}},
			}},
		},
	}}
	_, _ = c.ListApps(ctx)
	_, _ = c.GetApp(ctx, "web")
	_, _ = c.CreateApp(ctx, app)
	_, _ = c.UpdateApp(ctx, app)
	_ = c.DeleteApp(ctx, "web")
	_, _ = c.AppLogs(ctx, "web", LogOptions{Pod: "p", Container: "c", TailLines: 10, SinceSeconds: 60, Previous: true})

	_, _ = c.ListInstances(ctx)
	_, _ = c.GetInstance(ctx, 100)
	_, _ = c.ListInstanceTemplates(ctx)
	_, _ = c.CreateInstance(ctx, CreateInstanceInput{Name: "vm", Template: "1", CPUs: 2, MemoryMB: 2048, Description: &text, RootDiskGB: 20,
		SSHKeyIDs: []string{"k"}, SSHKey: &text, Password: &text, AdditionalVolumes: []InstanceVolume{{SizeGB: 10, Type: DiskSSD}}})
	_ = c.StartInstance(ctx, 100)
	_ = c.StopInstance(ctx, 100)
	_ = c.RebootInstance(ctx, 100)
	_ = c.ResizeInstance(ctx, 100, 4, 4096)
	_ = c.ResizeInstanceDisk(ctx, 100, "scsi0", 40)
	_ = c.DeleteInstance(ctx, 100)

	pg := PostgresClusterInput{Name: "db", Spec: PostgresSpec{Instances: 2, Storage: PostgresStorage{Size: "10Gi"}, Resources: &Resources{Requests: &ResourceList{CPU: &text}}}}
	_, _ = c.ListPostgresClusters(ctx)
	_, _ = c.GetPostgresCluster(ctx, "db")
	_, _ = c.CreatePostgresCluster(ctx, pg)
	_, _ = c.UpdatePostgresCluster(ctx, pg)
	_ = c.DeletePostgresCluster(ctx, "db", true)
	_, _ = c.ListPostgresDatabases(ctx, "db")
	_, _ = c.CreatePostgresDatabase(ctx, PostgresDatabase{Cluster: "db", Name: "app", Owner: "app", ReclaimPolicy: &text})
	_, _ = c.UpdatePostgresDatabase(ctx, PostgresDatabase{Cluster: "db", Name: "app", Owner: "app"})
	_ = c.DeletePostgresDatabase(ctx, "db", "app")
	_, _ = c.GetPostgresBackups(ctx, "db")
	_, _ = c.EnablePostgresBackups(ctx, "db", "30d")
	_, _ = c.SetPostgresBackupSchedule(ctx, "db", "0 3 * * *", "30d", false)
	_ = c.RemovePostgresBackupSchedule(ctx, "db")
	_, _ = c.BackupPostgresCluster(ctx, "db")
	_, _ = c.RestorePostgresCluster(ctx, RestorePostgresInput{Name: "db2", SourceCluster: "db", Backup: &text, TargetTime: &text, Instances: &n})

	_, _ = c.ListBuckets(ctx)
	_, _ = c.CreateBucket(ctx, "assets")
	_ = c.DeleteBucket(ctx, "assets")
	_, _ = c.ObjectStorageInfo(ctx)
	_, _ = c.ListStorageKeys(ctx)
	_, _ = c.CreateStorageKey(ctx, "ci")
	_ = c.DeleteStorageKey(ctx, "ci")
	_, _ = c.ListBucketNotifications(ctx, "assets")
	_, _ = c.CreateBucketNotification(ctx, BucketNotificationInput{Bucket: "assets", Events: []string{"CREATED"}, KeyPrefix: "in/", KeySuffix: ".jpg", Queue: "uploads"})
	_, _ = c.CreateBucketNotification(ctx, BucketNotificationInput{Bucket: "assets", Events: []string{"CREATED"}, Topic: "files", RoutingKey: "uploaded"})
	_ = c.DeleteBucketNotification(ctx, "assets", "id")

	_, _ = c.QueuesInfo(ctx)
	_, _ = c.ListQueues(ctx)
	_, _ = c.GetQueue(ctx, "orders")
	_, _ = c.CreateQueue(ctx, "orders")
	_ = c.PurgeQueue(ctx, "orders")
	_ = c.DeleteQueue(ctx, "orders")
	_, _ = c.ListTopics(ctx)
	_, _ = c.CreateTopic(ctx, "events")
	_ = c.DeleteTopic(ctx, "events")
	_, _ = c.ListSubscriptions(ctx, "events")
	_, _ = c.SubscribeQueue(ctx, "events", "orders", "order.*")
	_ = c.DeleteSubscription(ctx, "id")
	_, _ = c.ListQueueCredentials(ctx)
	_, _ = c.CreateQueueCredential(ctx, "worker")
	_ = c.DeleteQueueCredential(ctx, "worker")

	enabled := true
	_, _ = c.ListWebhooks(ctx)
	_, _ = c.GetWebhook(ctx, "hook")
	_, _ = c.CreateWebhook(ctx, WebhookInput{Name: "hook", Topic: "events", Filter: "order.*", URL: "https://example.com/hook", Secret: "s"})
	_, _ = c.CreateWebhook(ctx, WebhookInput{Name: "hook", Queue: "orders", URL: "https://example.com/hook"})
	_, _ = c.UpdateWebhook(ctx, "hook", WebhookUpdate{URL: &text, Secret: &text, Headers: map[string]string{"a": "b"}, ReplaceHeaders: true, Enabled: &enabled})
	_ = c.ResumeWebhook(ctx, "hook")
	_ = c.DeleteWebhook(ctx, "hook")
	_, _ = c.ListScheduledMessages(ctx)
	_, _ = c.GetScheduledMessage(ctx, "tick")
	message := ScheduledMessageInput{Name: "tick", Schedule: "*/5 * * * *", Queue: "orders", Body: "{}", ContentType: "application/json", Enabled: true}
	_, _ = c.CreateScheduledMessage(ctx, message)
	message.Queue, message.Topic, message.RoutingKey = "", "events", "tick"
	_, _ = c.UpdateScheduledMessage(ctx, message)
	_ = c.RunScheduledMessage(ctx, "tick")
	_ = c.DeleteScheduledMessage(ctx, "tick")

	job := ScheduledJobInput{Name: "report", Image: "registry.example.com/report:1", Command: []string{"run"}, Args: []string{"--all"},
		Env: []JobEnv{{Name: "A", Value: "b"}}, ImagePullSecrets: []string{"pull"}, CPU: 0.5, MemoryMB: 256}
	_, _ = c.ListScheduledJobs(ctx)
	_, _ = c.GetScheduledJob(ctx, "report")
	_, _ = c.CreateScheduledJob(ctx, job)
	_, _ = c.UpdateScheduledJob(ctx, job)
	_ = c.SetScheduledJobSuspended(ctx, "report", true)
	_, _ = c.RunScheduledJob(ctx, "report")
	_, _ = c.ListScheduledJobRuns(ctx, "report", 5)
	_, _ = c.ScheduledJobLogs(ctx, "report", "run-1", LogOptions{Pod: "p", TailLines: 10, SinceSeconds: 60})
	_ = c.DeleteScheduledJob(ctx, "report")

	_, _ = c.ExternalServicesPage(ctx, ListOptions{Limit: 10, Skip: 10, Search: "a", Sort: "name", Descending: true})
	_, _ = c.GetExternalService(ctx, "vm")
	_, _ = c.CreateExternalService(ctx, ExternalService{Name: "vm", ExternalName: "vm.example.com", Ports: []ServicePort{{Name: &text, Port: 80, TargetPort: 8080, Protocol: "TCP"}}})
	_ = c.DeleteExternalService(ctx, "vm")
	_, _ = c.IngressRoutesPage(ctx, ListOptions{Limit: 10})
	_, _ = c.GetIngressRoute(ctx, "shop")
	_, _ = c.CreateIngressRoute(ctx, IngressRoute{Name: "shop", EntryPoints: []string{"e"}, CertResolver: &text,
		Routes: []RouteRule{{Match: "Host(`shop.example.com`)", Middlewares: []string{"auth"}, Services: []RouteService{{Name: "vm", Port: 80, Scheme: &text}}}}})
	_ = c.DeleteIngressRoute(ctx, "shop")
	_, _ = c.MiddlewaresPage(ctx, ListOptions{Limit: 10})
	_, _ = c.GetMiddleware(ctx, "auth")
	_, _ = c.CreateMiddleware(ctx, Middleware{Name: "auth", Type: MiddlewareBasicAuth, BasicAuth: &BasicAuth{Username: "u", Password: "p"}})
	_, _ = c.CreateMiddleware(ctx, Middleware{Name: "r", Type: MiddlewareRedirectRegex, RedirectRegex: &RedirectRegex{Regex: "a", Replacement: "b", Permanent: true}})
	_, _ = c.CreateMiddleware(ctx, Middleware{Name: "s", Type: MiddlewareRedirectScheme, RedirectScheme: &RedirectScheme{Scheme: "https", Port: &text}})
	_, _ = c.CreateMiddleware(ctx, Middleware{Name: "p", Type: MiddlewareStripPrefix, StripPrefix: &StripPrefix{Prefixes: []string{"/api"}}})
	_ = c.DeleteMiddleware(ctx, "auth")

	_, _ = c.DNSAvailable(ctx)
	_, _ = c.ListDNSZones(ctx)
	_, _ = c.ListDNSRecords(ctx, "example.com")
	_, _ = c.GetDNSRecord(ctx, "id")
	_, _ = c.CreateDNSRecord(ctx, "example.com", "shop.example.com", DNSRecordCNAME, "edge.example.net")
	_, _ = c.UpdateDNSRecord(ctx, "id", "other.example.net")
	_ = c.DeleteDNSRecord(ctx, "id")

	_, _ = c.RegistriesPage(ctx, ListOptions{Limit: 10})
	_, _ = c.GetRegistry(ctx, "images")
	_, _ = c.CreateRegistry(ctx, "images")
	_, _ = c.IssueRegistryCredentials(ctx, "images", true, time.Hour)
	_ = c.DeleteRegistry(ctx, "images")
	_, _ = c.ListSecrets(ctx)
	_, _ = c.GetSecret(ctx, "conf")
	_, _ = c.CreateSecret(ctx, "conf", map[string]string{"a": "b"})
	_, _ = c.UpdateSecret(ctx, "conf", map[string]string{"a": "c"})
	_ = c.DeleteSecret(ctx, "conf")
	_, _ = c.ListSSHKeys(ctx)
	_, _ = c.GetSSHKey(ctx, "id")
	_, _ = c.AddSSHKey(ctx, "laptop", "ssh-ed25519 AAAA")
	_, _ = c.RenameSSHKey(ctx, "id", "desk")
	_ = c.DeleteSSHKey(ctx, "id")
	_, _ = c.AddPersonalSSHKey(ctx, "laptop", "ssh-ed25519 AAAA")
	_, _ = c.RenamePersonalSSHKey(ctx, "id", "desk")
	_ = c.DeletePersonalSSHKey(ctx, "id")
	_, _ = c.ListVolumes(ctx)
	_, _ = c.CreateVolume(ctx, "data", 10)
	_ = c.DeleteVolume(ctx, "data")
	_, _ = c.CreateVPC(ctx, "main", "198.51.100.0/24")

	_, _ = c.ListServiceAccounts(ctx)
	_, _ = c.CreateServiceAccount(ctx, "ci", time.Now().Add(time.Hour))
	_, _ = c.RotateServiceAccountSecret(ctx, "id")
	_, _ = c.SetServiceAccountActive(ctx, "id", false)
	_ = c.DeleteServiceAccount(ctx, "id")
	roles, permissions, restricted := []string{"viewer"}, []string{"apps.read"}, true
	scoped := []ScopedAccess{{ResourceType: "queue", Selector: Selector{Names: []string{"orders"}, Prefixes: []string{"o-"}, Labels: []KeyValue{{Key: "team", Value: "a"}}}, Roles: roles, Permissions: permissions}}
	_, _ = c.ListPrincipals(ctx, "apps.read")
	_, _ = c.SetPrincipalAccess(ctx, AccessInput{Username: "ci", Roles: &roles, Permissions: &permissions, Scoped: &scoped, Restricted: &restricted})
	_, _ = c.GrantPermissions(ctx, "ci", permissions)
	_, _ = c.RevokePermissions(ctx, "ci", permissions)
	_, _ = c.ListResourceTypes(ctx)
	role := Role{Name: "deployer", Description: "d", Permissions: permissions}
	_, _ = c.ListRoles(ctx)
	_ = c.CreateRole(ctx, role)
	_ = c.UpdateRole(ctx, role)
	_ = c.DeleteRole(ctx, "deployer")
	_, _ = c.GetResourceLabels(ctx, "queue", "orders")
	_ = c.SetResourceLabels(ctx, "queue", "orders", map[string]string{"team": "a"})

	pool := NodePool{Name: "workers", Replicas: 2, VCPU: 2, RAMGB: 4, DiskGB: 40}
	_, _ = c.ListKubernetesClusters(ctx)
	_, _ = c.GetKubernetesCluster(ctx, "k")
	_, _ = c.CreateKubernetesCluster(ctx, KubernetesClusterInput{Name: "k", Version: "v1", Pools: []NodePool{pool}})
	_, _ = c.ScaleNodePool(ctx, "k", "workers", 3)
	_, _ = c.AddNodePool(ctx, "k", pool)
	_, _ = c.RemoveNodePool(ctx, "k", "workers")
	_, _ = c.UpgradeKubernetesCluster(ctx, "k", "v2")
	_, _ = c.Kubeconfig(ctx, "k", KubeconfigSession)
	_, _ = c.ClusterKubeconfig(ctx, "k")
	_, _ = c.ClusterToken(ctx, "k", 600)
	_ = c.DeleteKubernetesCluster(ctx, "k")

	_, _ = c.ListAIModels(ctx)
	_, _ = c.WaitForTask(ctx, "task", time.Millisecond)
}
