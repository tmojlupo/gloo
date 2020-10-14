package test

var awsFmtString = `
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy"
stats_sinks:
- name: envoy.stat_sinks.metrics_service
  typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9966}
static_resources:
  listeners: # if or $statsConfig.enabled (or $spec.readConfig $spec.extraListenersHelper) # $spec.extraListenersHelper
  - name: prometheus_listener
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          stat_prefix: prometheus
          route_config:
            name: prometheus_route
            virtual_hosts:
            - name: prometheus_host
              domains:
              - "*"
              routes:
              - match:
                  path: "/ready"
                  headers:
                  - name: ":method"
                    exact_match: GET
                route:
                  cluster: admin_port_cluster
              - match:
                  prefix: "/metrics"
                  headers:
                  - name: ":method"
                    exact_match: GET
                route:
                  prefix_rewrite: "/stats/prometheus"
                  cluster: admin_port_cluster
          http_filters:
          - name: envoy.filters.http.router # if $spec.tracing # if $statsConfig.enabled # if $spec.readConfig
  clusters:
  - name: gloo.gloo-system.svc.cluster.local:9977
    alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    http2_protocol_options: {}
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
    respect_dns_ttl: true
  - name: rest_xds_cluster
    alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
    respect_dns_ttl: true
  - name: wasm-cache
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
    respect_dns_ttl: true
  - name: gloo.gloo-system.svc.cluster.local:9966
    alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    http2_protocol_options: {}
    type: STRICT_DNS # if .Values.accessLogger.enabled # if $spec.tracing
  - name: aws_sts_cluster
    connect_timeout: 5.000s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        sni: sts.%samazonaws.com
    load_assignment:
      cluster_name: aws_sts_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                port_value: 443
                address: sts.%samazonaws.com # if $.Values.settings.aws.enableServiceAccountCredentials
  - name: admin_port_cluster
    connect_timeout: 5.000s
    type: STATIC
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000 # if or $statsConfig.enabled ($spec.readConfig)

dynamic_resources:
  ads_config:
    api_type: GRPC
    rate_limit_settings: {}
    grpc_services:
    - envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9977}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000 # if (empty $spec.configMap.data) ## allows full custom # range $name, $spec := .Values.gatewayProxies# if .Values.gateway.enabled
`

var confWithoutTracing = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    name: rest_xds_cluster
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    name: wasm-cache
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.filters.network.http_connection_manager
    name: prometheus_listener
stats_sinks:
- typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.stat_sinks.metrics_service
`

var confWithTracingProvider = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    name: rest_xds_cluster
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    name: wasm-cache
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
          tracing:
            provider:
              another: line
              trace: spec
        name: envoy.filters.network.http_connection_manager
    name: prometheus_listener
stats_sinks:
- typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.stat_sinks.metrics_service
`

var confWithTracingProviderCluster = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    name: rest_xds_cluster
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    name: wasm-cache
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 1s
    lb_policy: round_robin
    load_assignment:
      cluster_name: zipkin
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: zipkin
                port_value: 1234
    name: zipkin
    respect_dns_ttl: true
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
          tracing:
            provider:
              typed_config:
                '@type': type.googleapis.com/envoy.config.trace.v2.ZipkinConfig
                collector_cluster: zipkin
                collector_endpoint: /api/v2/spans
        name: envoy.filters.network.http_connection_manager
    name: prometheus_listener
stats_sinks:
- typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.stat_sinks.metrics_service
`

var confWithReadConfig = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    name: rest_xds_cluster
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    name: wasm-cache
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.filters.network.http_connection_manager
    name: prometheus_listener
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8082
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: read_config_route
            virtual_hosts:
            - domains:
              - '*'
              name: read_config_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /stats
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /config_dump
                route:
                  cluster: admin_port_cluster
          stat_prefix: read_config
        name: envoy.filters.network.http_connection_manager
    name: read_config_listener
stats_sinks:
- typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.stat_sinks.metrics_service
`

var confWithAccessLogger = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      overload:
        global_downstream_max_connections: 250000
  - name: admin_layer
    admin_layer: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9976
    name: rest_xds_cluster
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - connect_timeout: 5.000s
    load_assignment:
      cluster_name: wasm-cache
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9979
    name: wasm-cache
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: access_log_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gateway-proxy-access-logger.gloo-system.svc.cluster.local
                port_value: 8083
    name: access_log_cluster
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          http_filters:
          - name: envoy.filters.http.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.filters.network.http_connection_manager
    name: prometheus_listener
stats_sinks:
- typed_config:
    "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.stat_sinks.metrics_service
`
