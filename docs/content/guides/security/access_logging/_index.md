---
title: Access Logging
weight: 40
description: Produce an access log representing traffic passing through the proxy
---

A common use case for the API gateway is to produce an **access log** (sometimes referred to as an audit log). The entries of an access log represent traffic through the proxy. The access log entries can be customized to include 
data from the request, the routing destination, and the response. The proxy can be configured to output multiple access logs with different configuration.

An access log may be written locally to a file or the `stdout` pipe in the proxy container, or it can be exported to a gRPC server for custom handling.

With Gloo Edge (starting in `0.18.1`), access logs can be enabled and customized per Envoy listener by modifying the **Gateway** Custom Resource.

### Data Available for Logging

Envoy exposes a lot of data that can be used when customizing access logs. Some common data properties available for both 
TCP and HTTP access logging include:
* The downstream (client) address, connection info, tls configuration, and timing
* The upstream (service) address, connection info, tls configuration, timing, and envoy routing information
* Relevant envoy configuration, such as rate of sampling (if used)
* Filter-specific context published to Envoy's dynamic metadata during the filter chain

#### Additional HTTP Properties

When Envoy is used as an HTTP proxy a large amount of additional HTTP information is available for access logging, including:
* Request data including the method, path, scheme, port, user agent, headers, body, and more 
* Response data including the response code, headers, body, and trailers, as well as a string representation of the response code
* Protocol version

#### Additional TCP Properties

Due to the limitations of the TCP protocol, the only additional data available for access logging when using Envoy as a TCP 
proxy are connection properties: bytes received and sent. 

### File-based Access Logging

File-based access logging can be enabled in Gloo Edge by customizing the **Gateway** CRD. For file-based access logs (also referred to 
as file sink access logs), there are two main configurations to set:
* Path: This is where the logs are written, either to `/dev/stdout` or to a file that is in a writable volume in the proxy container 
* Format: This provides a log template for either string or json-formatted logs

#### Outputting formatted strings

To configure access logs on a specific Envoy listener that output string-formatted logs to a file, 
we can add an access logging option to the corresponding Gateway CRD. 

For example, here is an example Gateway configuration that logs all requests into the HTTP port to 
standard out, using the [default string format](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#config-access-log-default-format):
```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
```

This will cause requests to the HTTP port to be logged to the application logs of the `gateway-proxy` deployment:
```
$ kubectl logs -n gloo-system deploy/gateway-proxy
```

```
[2020-03-17T18:39:54.919Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 2 2 "-" "curl/7.54.0" "eb5af05f-f14f-467b-994c-aeac9b5383f1" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:40:22.086Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 2 1 "-" "curl/7.54.0" "a90bb140-bf4a-42d7-84f3-bb983ae089ec" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:40:31.043Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 2 2 "-" "curl/7.54.0" "d10e4cc3-c148-4bcc-9c5d-b9b9b6cbf950" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:40:33.680Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 1 1 "-" "curl/7.54.0" "fe904bec-d2ba-4027-9aa3-fef74dbf927e" "35.196.131.38" "10.52.0.54:8080"
```

#### Customizing the string format

In the example above, the default string format was used. Alternatively, a custom string format can be provided explicitly:
```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: >
            [%START_TIME%] "%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%"
            %RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION%
            %RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% "%REQ(X-FORWARDED-FOR)%" "%REQ(USER-AGENT)%"
            "%REQ(X-REQUEST-ID)%" "%REQ(:AUTHORITY)%" "%UPSTREAM_HOST%"
```

This will cause requests to the HTTP port to be logged to the application logs of the `gateway-proxy` deployment:
```
$ kubectl logs -n gloo-system deploy/gateway-proxy
```

```
[2020-03-17T18:49:50.298Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 2 2 "-" "curl/7.54.0" "6fce19d1-63a9-438b-94ac-b4ec212b9027" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:49:52.141Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 1 0 "-" "curl/7.54.0" "ff80348a-5b69-4485-ac73-73f3fe825532" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:49:53.685Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 1 0 "-" "curl/7.54.0" "2a66949f-55e5-4dae-a03c-5f749ce4867c" "35.196.131.38" "10.52.0.54:8080"
[2020-03-17T18:49:55.191Z] "GET /sample-route-1 HTTP/1.1" 200 - 0 86 1 1 "-" "curl/7.54.0" "4a0013d3-f0e6-44b8-ad5f-fa181852e6cd" "35.196.131.38" "10.52.0.54:8080"
```

For more details about the Envoy string format, check out the [envoy docs](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#config-access-log-format-strings). 

#### Outputting structured json

Instead of outputting strings, the file sink access logger can be configured to log structured json instead. When 
configuring structured json, the Envoy fields are referenced in the same way as in the string format, however a mapping to json 
keys is defined: 

```yaml
apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     proxyNames:
       - gateway-proxy
     httpGateway: {}
     useProxyProto: false
     options:
       accessLoggingService:
         accessLog:
           - fileSink:
               path: /dev/stdout
               jsonFormat:
                 protocol: "%PROTOCOL%"
                 duration: "%DURATION%"
                 upstreamCluster: "%UPSTREAM_CLUSTER%"
                 upstreamHost: "%UPSTREAM_HOST%"
```

This will cause requests to the HTTP port to be logged to the application logs of the `gateway-proxy` deployment:
```
$ kubectl logs -n gloo-system deploy/gateway-proxy
```

```
{"protocol":"HTTP/1.1","upstreamHost":"10.52.0.54:8080","duration":"4","upstreamCluster":"default-petstore-8080_gloo-system"}
{"upstreamHost":"10.52.0.54:8080","duration":"3","upstreamCluster":"default-petstore-8080_gloo-system","protocol":"HTTP/1.1"}
{"protocol":"HTTP/1.1","upstreamHost":"10.52.0.54:8080","duration":"3","upstreamCluster":"default-petstore-8080_gloo-system"}
```

For more information about json format dictionaries, check out the [Envoy docs](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#format-dictionaries).

#### Outputting to a custom file

Instead of outputting the string or json-formatted access logs to standard out, it may be preferable to log them to 
a file local to the container. This requires a volume that is writable in the `gateway-proxy` container.

We'll update the path in our file sink to write to a file in the `/dev` directory, which is already writable:

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames:
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
        - fileSink:
            path: /dev/access-logs.json
            jsonFormat:
              protocol: "%PROTOCOL%"
              duration: "%DURATION%"
              upstreamCluster: "%UPSTREAM_CLUSTER%"
              upstreamHost: "%UPSTREAM_HOST%"
```

This will cause requests to the HTTP port to be logged to a file inside the `gateway-proxy` container:
```
$ kubectl exec -n gloo-system -it deploy/gateway-proxy -- cat /dev/access-logs.json
```

```
{"duration":"4","upstreamCluster":"default-petstore-8080_gloo-system","protocol":"HTTP/1.1","upstreamHost":"10.52.0.54:8080"}
{"upstreamCluster":"default-petstore-8080_gloo-system","protocol":"HTTP/1.1","upstreamHost":"10.52.0.54:8080","duration":"1"}
{"upstreamCluster":"default-petstore-8080_gloo-system","protocol":"HTTP/1.1","upstreamHost":"10.52.0.54:8080","duration":"1"}
```

### gRPC Access Logging

The previous section reviewed the different ways you can configure access logging to output to a file local to the 
proxy container. Alternatively, it may be desirable to configure Envoy to emit access logs to a gRPC endpoint. This would be
a custom service deployed to your cluster that receives access log events and then does something with them - such 
as writing them to a file in the access log gRPC service container, or sending them to an enterprise logging backend.

#### Deploying the open source gRPC access logger

Open source Gloo Edge includes an optional gRPC access log server implementation that can be turned on and deployed using
the following helm values:

```yaml
# Note: for enterprise users, this should be prefixed with "gloo"
# gloo:
#   accessLogger:
#     enabled: true

accessLogger:
  enabled: true
```

This will add a deployment to the `gloo-system` namespace called `gateway-proxy-access-logger`. It also deploys a 
Kubernetes service, and adds the following static cluster to the `gateway-proxy-envoy-config` config map:

```yaml
...
      - name: access_log_cluster
        connect_timeout: 5.000s
        load_assignment:
            cluster_name: access_log_cluster
            endpoints:
            - lb_endpoints:
              - endpoint:
                    address:
                        socket_address:
                            address: gateway-proxy-access-logger.gloo-system.svc.cluster.local
                            port_value: 8083
        http2_protocol_options: {}
        type: STRICT_DNS # if .Values.accessLogger.enabled # if $spec.tracing
...
```

This access logging service can now be used by configuring the **gateway** CRD:

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  proxyNames:
  - gateway-proxy
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
        - grpcService:
            logName: example
            staticClusterName: access_log_cluster
```

{{% notice note %}}
You may want to add additional configuration for the {{% protobuf name="als.options.gloo.solo.io.GrpcService" display="GrpcService"%}} such as additional request headers to log so they can be accessed in the gRPC access logs.
{{% /notice %}}

This will cause requests to the HTTP port to be logged to standard out in the `gateway-proxy-access-logger` container:
```
$ kubectl logs -n gloo-system deploy/gateway-proxy-access-logger
```

```
{"level":"info","ts":"2020-03-18T17:22:03.976Z","logger":"access_log","caller":"runner/run.go:92","msg":"Starting access-log server"}
{"level":"info","ts":"2020-03-18T17:22:03.977Z","logger":"access_log","caller":"runner/run.go:98","msg":"access-log server running in [gRPC] mode, listening at [:8083]"}
{"level":"info","ts":"2020-03-18T20:18:09.390Z","logger":"access_log","caller":"loggingservice/server.go:37","msg":"received access log message","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > "}
{"level":"info","ts":"2020-03-18T20:18:09.390Z","logger":"access_log","caller":"runner/run.go:48","msg":"received http request","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > ","protocol_version":"HTTP11","request_path":"/","request_method":"GET","response_status":"value:403 "}
{"level":"info","ts":"2020-03-18T20:18:11.401Z","logger":"access_log","caller":"loggingservice/server.go:37","msg":"received access log message","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > "}
{"level":"info","ts":"2020-03-18T20:18:11.401Z","logger":"access_log","caller":"runner/run.go:48","msg":"received http request","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > ","protocol_version":"HTTP11","request_path":"/","request_method":"GET","response_status":"value:403 "}
{"level":"info","ts":"2020-03-18T20:18:12.400Z","logger":"access_log","caller":"loggingservice/server.go:37","msg":"received access log message","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > "}
{"level":"info","ts":"2020-03-18T20:18:12.400Z","logger":"access_log","caller":"runner/run.go:48","msg":"received http request","logger_name":"example","node_id":"gateway-proxy-548b6587cb-hh6v2.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"fields:<key:\"role\" value:<string_value:\"gloo-system~gateway-proxy\" > > ","protocol_version":"HTTP11","request_path":"/","request_method":"GET","response_status":"value:403 "}
```

The code for this server implementation is available [here](https://github.com/solo-io/gloo/tree/master/projects/accesslogger). 

#### Building a custom service

If you are building a custom access logging gRPC service, you will need get it deployed alongside Gloo Edge. The Envoy
config (that Gloo Edge stores in `gateway-proxy-envoy-config`) will need to include a new static cluster pointing to your 
custom access log server. Once you have a named static cluster in your envoy config, you can reference it in 
your **gateway** CRD. 

The Gloo Edge access logger was written to be customizable with callbacks, so it may provide a useful starting point. Feel free
to open an issue in the Gloo Edge repo to track improvements to the existing implementation. 

To verify your Envoy access logging configuration, use `glooctl check`. If there is a problem configuring the Envoy 
listener with your custom access logging server, it should be reported there. 

### Configuring multiple access logs 

More than one access log can be configured for a single Envoy listener. Putting the examples above together, here is a configuration
that includes four different access log outputs: a default string-formatted access log to standard out on the Envoy container, a default
string-formatted access log to a file in the Envoy container, a json-formatted access log to a different file in the Envoy container, 
and a json-formatted access log to standard out in the `gateway-proxy-access-logger` container. 

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  proxyNames:
    - gateway-proxy
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
        - fileSink:
            path: /dev/stdout
            stringFormat: ""
        - fileSink:
            path: /dev/default-access-log.txt
            stringFormat: ""
        - fileSink:
            path: /dev/other-access-log.json
            jsonFormat:
              protocol: "%PROTOCOL%"
              duration: "%DURATION%"
              upstreamCluster: "%UPSTREAM_CLUSTER%"
              upstreamHost: "%UPSTREAM_HOST%"
        - grpcService:
            logName: example
            staticClusterName: access_log_cluster
```
