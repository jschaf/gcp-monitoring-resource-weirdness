package main

import (
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"context"
	"flag"
	"fmt"
	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/label"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

var (
	projectID         = flag.String("project-id", "", "GCP project ID")
	monitoredResource = flag.String("monitored-resource", "generic_task", "one of generic_task; none; k8s_container")
)

const (
	metricType = "custom.googleapis.com/test/histogram"
)

//goland:noinspection GoUnusedConst
const (
	Black Color = iota + 30
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

// Color represents a text color.
type Color uint8

// Add adds the coloring to the given string.
func (c Color) Add(s string) string { return fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(c), s) }

func main() {
	flag.Parse()
	err := runMain()
	if err != nil {
		slog.Error("run main", slog.Any("err", err))
		os.Exit(1)
	}
}

func runMain() error {
	// Validate flags.
	if *projectID == "" {
		return fmt.Errorf("--project-id is unset")
	}

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	metricClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return fmt.Errorf("create gcp monitoring client: %w", err)
	}

	metricDesc, err := createHistogramDescriptor(ctx, metricClient)
	if err != nil {
		return fmt.Errorf("create histogram descriptor: %w", err)
	}

	_, err = createHistogramTimeSeries(ctx, metricClient)
	if err != nil {
		return fmt.Errorf("create histogram time series: %w", err)
	}

	fmt.Printf("\n%s: showing CreateMetricDescriptor audit logs; sleeping\n", Blue.Add("[AUDIT LOGS]"))
	time.Sleep(5 * time.Second)
	cmd := exec.Command("gcloud", "logging", "read",
		fmt.Sprintf(`resource.type="audited_resource" resource.labels.service="monitoring.googleapis.com" resource.labels.method="google.monitoring.v3.MetricService.CreateMetricDescriptor" protoPayload.request.metricDescriptor.type="%s"`, metricType),
		"--limit=2",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("read audit logs: %w", err)
	}

	_, err = getMetricDescriptor(ctx, metricClient, metricDesc)
	if err != nil {
		return fmt.Errorf("get metric descriptor: %w", err)
	}

	return nil
}

func createHistogramDescriptor(ctx context.Context, client *monitoring.MetricClient) (*metricpb.MetricDescriptor, error) {
	desc, err := client.CreateMetricDescriptor(ctx, &monitoringpb.CreateMetricDescriptorRequest{
		Name: "projects/" + *projectID,
		MetricDescriptor: &metricpb.MetricDescriptor{
			Name: fmt.Sprintf("projects/%s/metricDescriptors/%s", *projectID, metricType),
			Type: metricType,
			Labels: []*label.LabelDescriptor{
				{Key: "key_a", ValueType: label.LabelDescriptor_STRING, Description: "some key a"},
			},
			MetricKind:  metricpb.MetricDescriptor_GAUGE,
			ValueType:   metricpb.MetricDescriptor_DISTRIBUTION,
			Unit:        "ms",
			Description: "test histogram",
			DisplayName: "Test Histogram Display name",
			MonitoredResourceTypes: []string{
				"generic_task",
				"k8s_container",
			},
			Metadata: nil, // not needed
			//LaunchStage: api.LaunchStage_GA, // optional, not needed
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create metric descriptor: %w", err)
	}

	fmt.Printf("\n%s\n%s\n", Blue.Add("[CREATED METRIC DESCRIPTOR]"), mustMarshalProtoText(desc))
	return desc, err
}

func mustMarshalProtoText(desc proto.Message) string {
	descTxt, err := prototext.MarshalOptions{Indent: "  "}.Marshal(desc)
	if err != nil {
		panic(fmt.Sprintf("marshal metric descriptor: %v", err))
	}
	return string(descTxt)
}

func getMetricDescriptor(ctx context.Context, client *monitoring.MetricClient, desc *metricpb.MetricDescriptor) (*metricpb.MetricDescriptor, error) {
	desc, err := client.GetMetricDescriptor(ctx, &monitoringpb.GetMetricDescriptorRequest{
		Name: desc.Name,
	})
	fmt.Printf("\n%s\n%s\n", Blue.Add("[GOT METRIC DESCRIPTOR]"), mustMarshalProtoText(desc))
	return desc, err
}

func createHistogramTimeSeries(ctx context.Context, metricClient *monitoring.MetricClient) (*monitoringpb.TimeSeries, error) {
	series := &monitoringpb.TimeSeries{
		Metric:     &metricpb.Metric{Type: metricType, Labels: map[string]string{"key_a": "value-a"}},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
		Points:     []*monitoringpb.Point{newHistogramPoint()},
		Unit:       "ms",
	}
	switch *monitoredResource {
	case "generic_task":
		series.Resource = &monitoredres.MonitoredResource{
			Type: "generic_task",
			Labels: map[string]string{
				"project_id": *projectID,
				"location":   "us-central1", // must be a GCP region or zone
				"namespace":  "test",
				"job":        "test-job",
				"task_id":    "test-task",
			},
		}
	case "k8s_container":
		series.Resource = &monitoredres.MonitoredResource{
			Type: "k8s_container",
			Labels: map[string]string{
				"project_id":     *projectID,
				"location":       "us-central1", // must be a GCP region or zone
				"cluster_name":   "test-cluster",
				"namespace_name": "test-namespace",
				"pod_name":       "test-pod",
				"container_name": "test-container",
			},
		}
	case "none":
		series.Resource = nil
	default:
		return nil, fmt.Errorf("unknown monitored resource: %s", *monitoredResource)
	}
	fmt.Printf("\n%s\n%s\n", Blue.Add("[CREATED TIME SERIES]"), mustMarshalProtoText(series))

	err := metricClient.CreateTimeSeries(ctx, &monitoringpb.CreateTimeSeriesRequest{
		Name:       "projects/" + *projectID,
		TimeSeries: []*monitoringpb.TimeSeries{series},
	})
	if err != nil {
		return nil, fmt.Errorf("create time series: %w", err)
	}
	return series, nil
}

func newHistogramPoint() *monitoringpb.Point {
	now := timestamppb.Now()
	return &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			EndTime:   now, // for gauge metrics, start must equal end
			StartTime: now,
		},
		Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DistributionValue{
			DistributionValue: &distribution.Distribution{
				Count:                 2,
				Mean:                  30,
				SumOfSquaredDeviation: 15,
				Range:                 nil, // GCP errors if set: "Distribution range is not supported"
				BucketOptions: &distribution.Distribution_BucketOptions{
					Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
						ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{
							Bounds: []float64{10, 50, 70},
						},
					},
				},
				BucketCounts: []int64{0, 1, 1, 0}, // len(Bounds) + buckets for explicit buckets
			},
		}},
	}
}
