package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_library(
    name = "go_default_library",
    srcs = [
        "device_plugin_stub.go",
        "device_run_container_options.go",
        "device_store.go",
        "endpoint.go",
        "endpoint_handler.go",
        "endpoint_store_shim.go",
        "manager.go",
        "manager_store.go",
        "manager_stub.go",
        "types.go",
        "utils.go",
    ],
    importpath = "k8s.io/kubernetes/pkg/kubelet/cm/devicemanager",
    deps = [
        "//pkg/apis/core/v1/helper:go_default_library",
        "//pkg/kubelet/apis/deviceplugin/v1alpha:go_default_library",
        "//pkg/kubelet/apis/pluginregistration/v1beta:go_default_library",
        "//pkg/kubelet/container:go_default_library",
        "//pkg/kubelet/lifecycle:go_default_library",
        "//pkg/kubelet/metrics:go_default_library",
        "//plugin/pkg/scheduler/schedulercache:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/grpc:go_default_library",
        "//vendor/k8s.io/api/core/v1:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "device_store_test.go",
        "endpoint_handler_test.go",
        "endpoint_store_shim_test.go",
        "endpoint_store_test.go",
        "endpoint_test.go",
        "manager_test.go",
    ],
    importpath = "k8s.io/kubernetes/pkg/kubelet/cm/devicemanager",
    library = ":go_default_library",
    deps = [
        "//pkg/kubelet/apis/deviceplugin/v1alpha:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/stretchr/testify/require:go_default_library",
        "//vendor/k8s.io/api/core/v1:go_default_library",
    ],
)
