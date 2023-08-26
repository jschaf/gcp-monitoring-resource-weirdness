# GCP Monitoring: Monitored Resource Weirdness

In my prod repo, it seems that adding a Monitored Resource to CreateTimeSeries
causes GCP Monitoring to drop the TimeSeries. It also seems impossible to set
the allowed monitored resources when calling CreateMetricDescriptor.

# References

* [Custom Metrics: GCP Monitored Resources][1]: For custom metrics, GCP
  Monitoring limits the monitored resource; includes `k8s_container` and
  `generic_task`.

* [GCP Monitoring API - CreateTimeSeries][2]

* [StackOverflow: One or more points were written more frequently than the maximum sampling period configured for the metric][3]
  The problem is that calling CreateTimeSeries without a set `resource` uses
  the global monitored resource. That means if you have multiple servers,
  they all update the same TimeSeries. Therefore, it's important to set the
  resource so that each server updates a separate TimeSeries.

[1]: https://cloud.google.com/monitoring/custom-metrics/creating-metrics#monitored_resources_for_custom_metrics
[2]: https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.timeSeries/create
[3]: https://stackoverflow.com/questions/58153208/one-or-more-points-were-written-more-frequently-than-the-maximum-sampling-period
