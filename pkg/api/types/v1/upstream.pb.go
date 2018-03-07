// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: upstream.proto

package v1

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"
import google_protobuf "github.com/gogo/protobuf/types"
import _ "github.com/golang/protobuf/ptypes/duration"
import _ "github.com/gogo/protobuf/gogoproto"

import time "time"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// *
// Upstream represents a destination for routing. Upstreams can be compared to
// [clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html?highlight=cluster) in Envoy terminology.
// Upstreams can take a variety of types<!--(TODO)--> in gloo. Language extensions known as plugins<!--(TODO)--> allow the addition of new
// types of upstreams. <!--See [upstream types](TODO) for a detailed description of available upstream types.-->
type Upstream struct {
	// Name of the upstream. Names must be unique and follow the following syntax rules:
	// One or more lowercase rfc1035/rfc1123 labels separated by '.' with a maximum length of 253 characters.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Type indicates the type of the upstream. Examples include service<!--(TODO)-->, kubernetes<!--(TODO)-->, and [aws](../plugins/aws.md)
	// Types are defined by the plugin<!--(TODO)--> associated with them.
	Type string `protobuf:"bytes,2,opt,name=type,proto3" json:"type,omitempty"`
	// Connection Timeout tells gloo to set a timeout for unresponsive connections created to this upstream.
	// If not provided by the user, it will set to a default value
	ConnectionTimeout time.Duration `protobuf:"bytes,3,opt,name=connection_timeout,json=connectionTimeout,stdduration" json:"connection_timeout"`
	// Spec contains properties that are specific to the upstream type. The spec is always required, but
	// the expected content is specified by the [upstream plugin] for the given upstream type.
	// Most often the upstream spec will be a map<string, string>
	Spec *google_protobuf.Struct `protobuf:"bytes,4,opt,name=spec" json:"spec,omitempty"`
	// Certain upstream types support (and may require) [functions](../introduction/concepts.md#Functions).
	// Functions allow function-level routing to be done. For example, the [AWS lambda](../plugins/aws.md) upstream type
	// Permits routing to AWS lambda function].
	// [routes](virtualhost.md#Route) on virtualhosts can specify function destinations to route to specific functions.
	Functions []*Function `protobuf:"bytes,5,rep,name=functions" json:"functions,omitempty"`
	// Status indicates the validation status of the upstream resource. Status is read-only by clients, and set by gloo during validation
	Status *Status `protobuf:"bytes,6,opt,name=status" json:"status,omitempty" testdiff:"ignore"`
	// Metadata contains the resource metadata for the upstream
	Metadata *Metadata `protobuf:"bytes,7,opt,name=metadata" json:"metadata,omitempty"`
}

func (m *Upstream) Reset()                    { *m = Upstream{} }
func (m *Upstream) String() string            { return proto.CompactTextString(m) }
func (*Upstream) ProtoMessage()               {}
func (*Upstream) Descriptor() ([]byte, []int) { return fileDescriptorUpstream, []int{0} }

func (m *Upstream) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Upstream) GetType() string {
	if m != nil {
		return m.Type
	}
	return ""
}

func (m *Upstream) GetConnectionTimeout() time.Duration {
	if m != nil {
		return m.ConnectionTimeout
	}
	return 0
}

func (m *Upstream) GetSpec() *google_protobuf.Struct {
	if m != nil {
		return m.Spec
	}
	return nil
}

func (m *Upstream) GetFunctions() []*Function {
	if m != nil {
		return m.Functions
	}
	return nil
}

func (m *Upstream) GetStatus() *Status {
	if m != nil {
		return m.Status
	}
	return nil
}

func (m *Upstream) GetMetadata() *Metadata {
	if m != nil {
		return m.Metadata
	}
	return nil
}

type Function struct {
	// Name of the function. Functions are referenced by name from routes and therefore must be unique within an upstream
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Spec for the function. Like [upstream specs](TODO), the content of function specs is specified by the [upstream plugin](TODO) for the upstream's type.
	Spec *google_protobuf.Struct `protobuf:"bytes,4,opt,name=spec" json:"spec,omitempty"`
}

func (m *Function) Reset()                    { *m = Function{} }
func (m *Function) String() string            { return proto.CompactTextString(m) }
func (*Function) ProtoMessage()               {}
func (*Function) Descriptor() ([]byte, []int) { return fileDescriptorUpstream, []int{1} }

func (m *Function) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Function) GetSpec() *google_protobuf.Struct {
	if m != nil {
		return m.Spec
	}
	return nil
}

func init() {
	proto.RegisterType((*Upstream)(nil), "v1.Upstream")
	proto.RegisterType((*Function)(nil), "v1.Function")
}
func (this *Upstream) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*Upstream)
	if !ok {
		that2, ok := that.(Upstream)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.Name != that1.Name {
		return false
	}
	if this.Type != that1.Type {
		return false
	}
	if this.ConnectionTimeout != that1.ConnectionTimeout {
		return false
	}
	if !this.Spec.Equal(that1.Spec) {
		return false
	}
	if len(this.Functions) != len(that1.Functions) {
		return false
	}
	for i := range this.Functions {
		if !this.Functions[i].Equal(that1.Functions[i]) {
			return false
		}
	}
	if !this.Status.Equal(that1.Status) {
		return false
	}
	if !this.Metadata.Equal(that1.Metadata) {
		return false
	}
	return true
}
func (this *Function) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*Function)
	if !ok {
		that2, ok := that.(Function)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.Name != that1.Name {
		return false
	}
	if !this.Spec.Equal(that1.Spec) {
		return false
	}
	return true
}

func init() { proto.RegisterFile("upstream.proto", fileDescriptorUpstream) }

var fileDescriptorUpstream = []byte{
	// 332 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x50, 0xc1, 0x4a, 0xeb, 0x40,
	0x14, 0x7d, 0x69, 0xf3, 0x6a, 0x3a, 0x2d, 0x85, 0x0e, 0x8a, 0x63, 0x91, 0xb6, 0x64, 0x15, 0x14,
	0x52, 0x5a, 0x57, 0xba, 0x2c, 0xe2, 0x46, 0xdc, 0x4c, 0x75, 0x2d, 0xd3, 0x74, 0x12, 0x02, 0x26,
	0x13, 0x32, 0x77, 0x0a, 0xfe, 0x89, 0x9f, 0xe0, 0x87, 0xb8, 0xf0, 0x2b, 0x2a, 0xf8, 0x09, 0x7e,
	0x81, 0xe4, 0x66, 0x6a, 0x41, 0xdd, 0xb8, 0x3b, 0x73, 0xce, 0x3d, 0xf7, 0x9e, 0x39, 0xa4, 0x67,
	0x0a, 0x0d, 0xa5, 0x14, 0x59, 0x58, 0x94, 0x0a, 0x14, 0x6d, 0xac, 0xa7, 0x83, 0xe3, 0x44, 0xa9,
	0xe4, 0x41, 0x4e, 0x90, 0x59, 0x9a, 0x78, 0xa2, 0xa1, 0x34, 0x11, 0xd4, 0x13, 0x83, 0xe1, 0x77,
	0x75, 0x65, 0x4a, 0x01, 0xa9, 0xca, 0xad, 0xbe, 0x9f, 0xa8, 0x44, 0x21, 0x9c, 0x54, 0xc8, 0xb2,
	0x5d, 0x0d, 0x02, 0x8c, 0xb6, 0xaf, 0x5e, 0x26, 0x41, 0xac, 0x04, 0x88, 0xfa, 0xed, 0xbf, 0x34,
	0x88, 0x77, 0x67, 0x83, 0x50, 0x4a, 0xdc, 0x5c, 0x64, 0x92, 0x39, 0x63, 0x27, 0x68, 0x73, 0xc4,
	0x15, 0x07, 0x8f, 0x85, 0x64, 0x8d, 0x9a, 0xab, 0x30, 0xe5, 0x84, 0x46, 0x2a, 0xcf, 0x65, 0x54,
	0x1d, 0xbf, 0x87, 0x34, 0x93, 0xca, 0x00, 0x6b, 0x8e, 0x9d, 0xa0, 0x33, 0x3b, 0x0a, 0xeb, 0x94,
	0xe1, 0x36, 0x65, 0x78, 0x69, 0x53, 0xce, 0xbd, 0xd7, 0xcd, 0xe8, 0xdf, 0xd3, 0xdb, 0xc8, 0xe1,
	0xfd, 0x9d, 0xfd, 0xb6, 0x76, 0xd3, 0x53, 0xe2, 0xea, 0x42, 0x46, 0xcc, 0xc5, 0x2d, 0x87, 0x3f,
	0xb6, 0x2c, 0xb0, 0x09, 0x8e, 0x43, 0xf4, 0x84, 0xb4, 0x63, 0x93, 0xa3, 0x5f, 0xb3, 0xff, 0xe3,
	0x66, 0xd0, 0x99, 0x75, 0xc3, 0xf5, 0x34, 0xbc, 0xb2, 0x24, 0xdf, 0xc9, 0xf4, 0x9c, 0xb4, 0xea,
	0x06, 0x58, 0x0b, 0x57, 0x93, 0x6a, 0x70, 0x81, 0xcc, 0xfc, 0xe0, 0x63, 0x33, 0xea, 0x83, 0xd4,
	0xb0, 0x4a, 0xe3, 0xf8, 0xc2, 0x4f, 0x93, 0x5c, 0x95, 0xd2, 0xe7, 0xd6, 0x40, 0x03, 0xe2, 0x6d,
	0xeb, 0x62, 0x7b, 0x68, 0xc6, 0x2b, 0x37, 0x96, 0xe3, 0x5f, 0xaa, 0x7f, 0x4d, 0xbc, 0xed, 0xed,
	0x5f, 0x5b, 0xfc, 0xcb, 0xef, 0xe6, 0xee, 0xf3, 0xfb, 0xd0, 0x59, 0xb6, 0x50, 0x3c, 0xfb, 0x0c,
	0x00, 0x00, 0xff, 0xff, 0xb6, 0xfc, 0x6d, 0xae, 0x28, 0x02, 0x00, 0x00,
}
