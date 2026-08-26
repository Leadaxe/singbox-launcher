package daemon

import (
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type LogLevel int32

const (
	LogLevel_PANIC LogLevel = 0
	LogLevel_FATAL LogLevel = 1
	LogLevel_ERROR LogLevel = 2
	LogLevel_WARN  LogLevel = 3
	LogLevel_INFO  LogLevel = 4
	LogLevel_DEBUG LogLevel = 5
	LogLevel_TRACE LogLevel = 6
)

// Enum value maps for LogLevel.
var (
	LogLevel_name = map[int32]string{
		0: "PANIC",
		1: "FATAL",
		2: "ERROR",
		3: "WARN",
		4: "INFO",
		5: "DEBUG",
		6: "TRACE",
	}
	LogLevel_value = map[string]int32{
		"PANIC": 0,
		"FATAL": 1,
		"ERROR": 2,
		"WARN":  3,
		"INFO":  4,
		"DEBUG": 5,
		"TRACE": 6,
	}
)

func (x LogLevel) Enum() *LogLevel {
	p := new(LogLevel)
	*p = x
	return p
}

func (x LogLevel) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LogLevel) Descriptor() protoreflect.EnumDescriptor {
	return file_daemon_started_service_proto_enumTypes[0].Descriptor()
}

func (LogLevel) Type() protoreflect.EnumType {
	return &file_daemon_started_service_proto_enumTypes[0]
}

func (x LogLevel) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LogLevel.Descriptor instead.
func (LogLevel) EnumDescriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{0}
}

type ConnectionEventType int32

const (
	ConnectionEventType_CONNECTION_EVENT_NEW    ConnectionEventType = 0
	ConnectionEventType_CONNECTION_EVENT_UPDATE ConnectionEventType = 1
	ConnectionEventType_CONNECTION_EVENT_CLOSED ConnectionEventType = 2
)

// Enum value maps for ConnectionEventType.
var (
	ConnectionEventType_name = map[int32]string{
		0: "CONNECTION_EVENT_NEW",
		1: "CONNECTION_EVENT_UPDATE",
		2: "CONNECTION_EVENT_CLOSED",
	}
	ConnectionEventType_value = map[string]int32{
		"CONNECTION_EVENT_NEW":    0,
		"CONNECTION_EVENT_UPDATE": 1,
		"CONNECTION_EVENT_CLOSED": 2,
	}
)

func (x ConnectionEventType) Enum() *ConnectionEventType {
	p := new(ConnectionEventType)
	*p = x
	return p
}

func (x ConnectionEventType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ConnectionEventType) Descriptor() protoreflect.EnumDescriptor {
	return file_daemon_started_service_proto_enumTypes[1].Descriptor()
}

func (ConnectionEventType) Type() protoreflect.EnumType {
	return &file_daemon_started_service_proto_enumTypes[1]
}

func (x ConnectionEventType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ConnectionEventType.Descriptor instead.
func (ConnectionEventType) EnumDescriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{1}
}

type USBDeviceState int32

const (
	USBDeviceState_USB_DEVICE_STATE_IDLE        USBDeviceState = 0
	USBDeviceState_USB_DEVICE_STATE_ATTACHED    USBDeviceState = 1
	USBDeviceState_USB_DEVICE_STATE_UNAVAILABLE USBDeviceState = 2
)

// Enum value maps for USBDeviceState.
var (
	USBDeviceState_name = map[int32]string{
		0: "USB_DEVICE_STATE_IDLE",
		1: "USB_DEVICE_STATE_ATTACHED",
		2: "USB_DEVICE_STATE_UNAVAILABLE",
	}
	USBDeviceState_value = map[string]int32{
		"USB_DEVICE_STATE_IDLE":        0,
		"USB_DEVICE_STATE_ATTACHED":    1,
		"USB_DEVICE_STATE_UNAVAILABLE": 2,
	}
)

func (x USBDeviceState) Enum() *USBDeviceState {
	p := new(USBDeviceState)
	*p = x
	return p
}

func (x USBDeviceState) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (USBDeviceState) Descriptor() protoreflect.EnumDescriptor {
	return file_daemon_started_service_proto_enumTypes[2].Descriptor()
}

func (USBDeviceState) Type() protoreflect.EnumType {
	return &file_daemon_started_service_proto_enumTypes[2]
}

func (x USBDeviceState) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use USBDeviceState.Descriptor instead.
func (USBDeviceState) EnumDescriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{2}
}

type USBBackend int32

const (
	USBBackend_USB_BACKEND_UNSPECIFIED     USBBackend = 0
	USBBackend_USB_BACKEND_LINUX_SYSFS     USBBackend = 1
	USBBackend_USB_BACKEND_DYNAMIC         USBBackend = 2
	USBBackend_USB_BACKEND_DARWIN_IOKIT    USBBackend = 3
	USBBackend_USB_BACKEND_WINDOWS_VBOXUSB USBBackend = 4
)

// Enum value maps for USBBackend.
var (
	USBBackend_name = map[int32]string{
		0: "USB_BACKEND_UNSPECIFIED",
		1: "USB_BACKEND_LINUX_SYSFS",
		2: "USB_BACKEND_DYNAMIC",
		3: "USB_BACKEND_DARWIN_IOKIT",
		4: "USB_BACKEND_WINDOWS_VBOXUSB",
	}
	USBBackend_value = map[string]int32{
		"USB_BACKEND_UNSPECIFIED":     0,
		"USB_BACKEND_LINUX_SYSFS":     1,
		"USB_BACKEND_DYNAMIC":         2,
		"USB_BACKEND_DARWIN_IOKIT":    3,
		"USB_BACKEND_WINDOWS_VBOXUSB": 4,
	}
)

func (x USBBackend) Enum() *USBBackend {
	p := new(USBBackend)
	*p = x
	return p
}

func (x USBBackend) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (USBBackend) Descriptor() protoreflect.EnumDescriptor {
	return file_daemon_started_service_proto_enumTypes[3].Descriptor()
}

func (USBBackend) Type() protoreflect.EnumType {
	return &file_daemon_started_service_proto_enumTypes[3]
}

func (x USBBackend) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use USBBackend.Descriptor instead.
func (USBBackend) EnumDescriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{3}
}

type ServiceStatus_Type int32

const (
	ServiceStatus_IDLE     ServiceStatus_Type = 0
	ServiceStatus_STARTING ServiceStatus_Type = 1
	ServiceStatus_STARTED  ServiceStatus_Type = 2
	ServiceStatus_STOPPING ServiceStatus_Type = 3
	ServiceStatus_FATAL    ServiceStatus_Type = 4
)

// Enum value maps for ServiceStatus_Type.
var (
	ServiceStatus_Type_name = map[int32]string{
		0: "IDLE",
		1: "STARTING",
		2: "STARTED",
		3: "STOPPING",
		4: "FATAL",
	}
	ServiceStatus_Type_value = map[string]int32{
		"IDLE":     0,
		"STARTING": 1,
		"STARTED":  2,
		"STOPPING": 3,
		"FATAL":    4,
	}
)

func (x ServiceStatus_Type) Enum() *ServiceStatus_Type {
	p := new(ServiceStatus_Type)
	*p = x
	return p
}

func (x ServiceStatus_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ServiceStatus_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_daemon_started_service_proto_enumTypes[4].Descriptor()
}

func (ServiceStatus_Type) Type() protoreflect.EnumType {
	return &file_daemon_started_service_proto_enumTypes[4]
}

func (x ServiceStatus_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ServiceStatus_Type.Descriptor instead.
func (ServiceStatus_Type) EnumDescriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{1, 0}
}

type Version struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Version       string                 `protobuf:"bytes,1,opt,name=version,proto3" json:"version,omitempty"`
	ApiVersion    int32                  `protobuf:"varint,2,opt,name=apiVersion,proto3" json:"apiVersion,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Version) Reset() {
	*x = Version{}
	mi := &file_daemon_started_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Version) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Version) ProtoMessage() {}

func (x *Version) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Version.ProtoReflect.Descriptor instead.
func (*Version) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{0}
}

func (x *Version) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

func (x *Version) GetApiVersion() int32 {
	if x != nil {
		return x.ApiVersion
	}
	return 0
}

type ServiceStatus struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Status        ServiceStatus_Type     `protobuf:"varint,1,opt,name=status,proto3,enum=daemon.ServiceStatus_Type" json:"status,omitempty"`
	ErrorMessage  string                 `protobuf:"bytes,2,opt,name=errorMessage,proto3" json:"errorMessage,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ServiceStatus) Reset() {
	*x = ServiceStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServiceStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServiceStatus) ProtoMessage() {}

func (x *ServiceStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServiceStatus.ProtoReflect.Descriptor instead.
func (*ServiceStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{1}
}

func (x *ServiceStatus) GetStatus() ServiceStatus_Type {
	if x != nil {
		return x.Status
	}
	return ServiceStatus_IDLE
}

func (x *ServiceStatus) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

type SubscribeStatusRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Interval      int64                  `protobuf:"varint,1,opt,name=interval,proto3" json:"interval,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubscribeStatusRequest) Reset() {
	*x = SubscribeStatusRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubscribeStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubscribeStatusRequest) ProtoMessage() {}

func (x *SubscribeStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubscribeStatusRequest.ProtoReflect.Descriptor instead.
func (*SubscribeStatusRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{2}
}

func (x *SubscribeStatusRequest) GetInterval() int64 {
	if x != nil {
		return x.Interval
	}
	return 0
}

type Log struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Messages      []*Log_Message         `protobuf:"bytes,1,rep,name=messages,proto3" json:"messages,omitempty"`
	Reset_        bool                   `protobuf:"varint,2,opt,name=reset,proto3" json:"reset,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Log) Reset() {
	*x = Log{}
	mi := &file_daemon_started_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Log) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Log) ProtoMessage() {}

func (x *Log) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Log.ProtoReflect.Descriptor instead.
func (*Log) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{3}
}

func (x *Log) GetMessages() []*Log_Message {
	if x != nil {
		return x.Messages
	}
	return nil
}

func (x *Log) GetReset_() bool {
	if x != nil {
		return x.Reset_
	}
	return false
}

type DefaultLogLevel struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Level         LogLevel               `protobuf:"varint,1,opt,name=level,proto3,enum=daemon.LogLevel" json:"level,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DefaultLogLevel) Reset() {
	*x = DefaultLogLevel{}
	mi := &file_daemon_started_service_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DefaultLogLevel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DefaultLogLevel) ProtoMessage() {}

func (x *DefaultLogLevel) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DefaultLogLevel.ProtoReflect.Descriptor instead.
func (*DefaultLogLevel) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{4}
}

func (x *DefaultLogLevel) GetLevel() LogLevel {
	if x != nil {
		return x.Level
	}
	return LogLevel_PANIC
}

type Status struct {
	state            protoimpl.MessageState `protogen:"open.v1"`
	Memory           uint64                 `protobuf:"varint,1,opt,name=memory,proto3" json:"memory,omitempty"`
	Goroutines       int32                  `protobuf:"varint,2,opt,name=goroutines,proto3" json:"goroutines,omitempty"`
	ConnectionsIn    int32                  `protobuf:"varint,3,opt,name=connectionsIn,proto3" json:"connectionsIn,omitempty"`
	ConnectionsOut   int32                  `protobuf:"varint,4,opt,name=connectionsOut,proto3" json:"connectionsOut,omitempty"`
	TrafficAvailable bool                   `protobuf:"varint,5,opt,name=trafficAvailable,proto3" json:"trafficAvailable,omitempty"`
	Uplink           int64                  `protobuf:"varint,6,opt,name=uplink,proto3" json:"uplink,omitempty"`
	Downlink         int64                  `protobuf:"varint,7,opt,name=downlink,proto3" json:"downlink,omitempty"`
	UplinkTotal      int64                  `protobuf:"varint,8,opt,name=uplinkTotal,proto3" json:"uplinkTotal,omitempty"`
	DownlinkTotal    int64                  `protobuf:"varint,9,opt,name=downlinkTotal,proto3" json:"downlinkTotal,omitempty"`
	unknownFields    protoimpl.UnknownFields
	sizeCache        protoimpl.SizeCache
}

func (x *Status) Reset() {
	*x = Status{}
	mi := &file_daemon_started_service_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Status) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Status) ProtoMessage() {}

func (x *Status) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Status.ProtoReflect.Descriptor instead.
func (*Status) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{5}
}

func (x *Status) GetMemory() uint64 {
	if x != nil {
		return x.Memory
	}
	return 0
}

func (x *Status) GetGoroutines() int32 {
	if x != nil {
		return x.Goroutines
	}
	return 0
}

func (x *Status) GetConnectionsIn() int32 {
	if x != nil {
		return x.ConnectionsIn
	}
	return 0
}

func (x *Status) GetConnectionsOut() int32 {
	if x != nil {
		return x.ConnectionsOut
	}
	return 0
}

func (x *Status) GetTrafficAvailable() bool {
	if x != nil {
		return x.TrafficAvailable
	}
	return false
}

func (x *Status) GetUplink() int64 {
	if x != nil {
		return x.Uplink
	}
	return 0
}

func (x *Status) GetDownlink() int64 {
	if x != nil {
		return x.Downlink
	}
	return 0
}

func (x *Status) GetUplinkTotal() int64 {
	if x != nil {
		return x.UplinkTotal
	}
	return 0
}

func (x *Status) GetDownlinkTotal() int64 {
	if x != nil {
		return x.DownlinkTotal
	}
	return 0
}

type Groups struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Group         []*Group               `protobuf:"bytes,1,rep,name=group,proto3" json:"group,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Groups) Reset() {
	*x = Groups{}
	mi := &file_daemon_started_service_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Groups) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Groups) ProtoMessage() {}

func (x *Groups) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Groups.ProtoReflect.Descriptor instead.
func (*Groups) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{6}
}

func (x *Groups) GetGroup() []*Group {
	if x != nil {
		return x.Group
	}
	return nil
}

type Group struct {
	state      protoimpl.MessageState `protogen:"open.v1"`
	Tag        string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	Type       string                 `protobuf:"bytes,2,opt,name=type,proto3" json:"type,omitempty"`
	Selectable bool                   `protobuf:"varint,3,opt,name=selectable,proto3" json:"selectable,omitempty"`
	// The group's current node. For a selector and for urltest mode: least_test this is
	// THE selected node. For urltest mode: round_robin there is no single current node —
	// the field carries the last node the balancer happened to pick, so treat it as a hint
	// and read the full rotation state via GetPool. lx: SPEC 019 v2.
	Selected string       `protobuf:"bytes,4,opt,name=selected,proto3" json:"selected,omitempty"`
	IsExpand bool         `protobuf:"varint,5,opt,name=isExpand,proto3" json:"isExpand,omitempty"`
	Items    []*GroupItem `protobuf:"bytes,6,rep,name=items,proto3" json:"items,omitempty"`
	// urltest mode: "least_test" | "round_robin". Empty for every non-urltest group
	// (selector), so it doubles as "is this group balanced at all" without probing GetPool
	// — which is gated behind the with_lx_command build tag. lx: SPEC 019 v2.
	Mode          string `protobuf:"bytes,7,opt,name=mode,proto3" json:"mode,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Group) Reset() {
	*x = Group{}
	mi := &file_daemon_started_service_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Group) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Group) ProtoMessage() {}

func (x *Group) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Group.ProtoReflect.Descriptor instead.
func (*Group) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{7}
}

func (x *Group) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *Group) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *Group) GetSelectable() bool {
	if x != nil {
		return x.Selectable
	}
	return false
}

func (x *Group) GetSelected() string {
	if x != nil {
		return x.Selected
	}
	return ""
}

func (x *Group) GetIsExpand() bool {
	if x != nil {
		return x.IsExpand
	}
	return false
}

func (x *Group) GetItems() []*GroupItem {
	if x != nil {
		return x.Items
	}
	return nil
}

func (x *Group) GetMode() string {
	if x != nil {
		return x.Mode
	}
	return ""
}

type GroupItem struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Tag           string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	Type          string                 `protobuf:"bytes,2,opt,name=type,proto3" json:"type,omitempty"`
	UrlTestTime   int64                  `protobuf:"varint,3,opt,name=urlTestTime,proto3" json:"urlTestTime,omitempty"`
	UrlTestDelay  int32                  `protobuf:"varint,4,opt,name=urlTestDelay,proto3" json:"urlTestDelay,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GroupItem) Reset() {
	*x = GroupItem{}
	mi := &file_daemon_started_service_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GroupItem) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GroupItem) ProtoMessage() {}

func (x *GroupItem) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GroupItem.ProtoReflect.Descriptor instead.
func (*GroupItem) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{8}
}

func (x *GroupItem) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *GroupItem) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *GroupItem) GetUrlTestTime() int64 {
	if x != nil {
		return x.UrlTestTime
	}
	return 0
}

func (x *GroupItem) GetUrlTestDelay() int32 {
	if x != nil {
		return x.UrlTestDelay
	}
	return 0
}

type URLTestRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OutboundTag   string                 `protobuf:"bytes,1,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *URLTestRequest) Reset() {
	*x = URLTestRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *URLTestRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*URLTestRequest) ProtoMessage() {}

func (x *URLTestRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use URLTestRequest.ProtoReflect.Descriptor instead.
func (*URLTestRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{9}
}

func (x *URLTestRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

type SelectOutboundRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	GroupTag      string                 `protobuf:"bytes,1,opt,name=groupTag,proto3" json:"groupTag,omitempty"`
	OutboundTag   string                 `protobuf:"bytes,2,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SelectOutboundRequest) Reset() {
	*x = SelectOutboundRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[10]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SelectOutboundRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SelectOutboundRequest) ProtoMessage() {}

func (x *SelectOutboundRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[10]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SelectOutboundRequest.ProtoReflect.Descriptor instead.
func (*SelectOutboundRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{10}
}

func (x *SelectOutboundRequest) GetGroupTag() string {
	if x != nil {
		return x.GroupTag
	}
	return ""
}

func (x *SelectOutboundRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

type SetGroupExpandRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	GroupTag      string                 `protobuf:"bytes,1,opt,name=groupTag,proto3" json:"groupTag,omitempty"`
	IsExpand      bool                   `protobuf:"varint,2,opt,name=isExpand,proto3" json:"isExpand,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetGroupExpandRequest) Reset() {
	*x = SetGroupExpandRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[11]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetGroupExpandRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetGroupExpandRequest) ProtoMessage() {}

func (x *SetGroupExpandRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[11]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetGroupExpandRequest.ProtoReflect.Descriptor instead.
func (*SetGroupExpandRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{11}
}

func (x *SetGroupExpandRequest) GetGroupTag() string {
	if x != nil {
		return x.GroupTag
	}
	return ""
}

func (x *SetGroupExpandRequest) GetIsExpand() bool {
	if x != nil {
		return x.IsExpand
	}
	return false
}

type ClashMode struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Mode          string                 `protobuf:"bytes,3,opt,name=mode,proto3" json:"mode,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClashMode) Reset() {
	*x = ClashMode{}
	mi := &file_daemon_started_service_proto_msgTypes[12]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClashMode) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClashMode) ProtoMessage() {}

func (x *ClashMode) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[12]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClashMode.ProtoReflect.Descriptor instead.
func (*ClashMode) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{12}
}

func (x *ClashMode) GetMode() string {
	if x != nil {
		return x.Mode
	}
	return ""
}

type ClashModeStatus struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ModeList      []string               `protobuf:"bytes,1,rep,name=modeList,proto3" json:"modeList,omitempty"`
	CurrentMode   string                 `protobuf:"bytes,2,opt,name=currentMode,proto3" json:"currentMode,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClashModeStatus) Reset() {
	*x = ClashModeStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[13]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClashModeStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClashModeStatus) ProtoMessage() {}

func (x *ClashModeStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[13]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClashModeStatus.ProtoReflect.Descriptor instead.
func (*ClashModeStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{13}
}

func (x *ClashModeStatus) GetModeList() []string {
	if x != nil {
		return x.ModeList
	}
	return nil
}

func (x *ClashModeStatus) GetCurrentMode() string {
	if x != nil {
		return x.CurrentMode
	}
	return ""
}

type SubscribeConnectionsRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Interval      int64                  `protobuf:"varint,1,opt,name=interval,proto3" json:"interval,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubscribeConnectionsRequest) Reset() {
	*x = SubscribeConnectionsRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[14]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubscribeConnectionsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubscribeConnectionsRequest) ProtoMessage() {}

func (x *SubscribeConnectionsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[14]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubscribeConnectionsRequest.ProtoReflect.Descriptor instead.
func (*SubscribeConnectionsRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{14}
}

func (x *SubscribeConnectionsRequest) GetInterval() int64 {
	if x != nil {
		return x.Interval
	}
	return 0
}

type ConnectionEvent struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Type          ConnectionEventType    `protobuf:"varint,1,opt,name=type,proto3,enum=daemon.ConnectionEventType" json:"type,omitempty"`
	Id            string                 `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	Connection    *Connection            `protobuf:"bytes,3,opt,name=connection,proto3" json:"connection,omitempty"`
	UplinkDelta   int64                  `protobuf:"varint,4,opt,name=uplinkDelta,proto3" json:"uplinkDelta,omitempty"`
	DownlinkDelta int64                  `protobuf:"varint,5,opt,name=downlinkDelta,proto3" json:"downlinkDelta,omitempty"`
	ClosedAt      int64                  `protobuf:"varint,6,opt,name=closedAt,proto3" json:"closedAt,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ConnectionEvent) Reset() {
	*x = ConnectionEvent{}
	mi := &file_daemon_started_service_proto_msgTypes[15]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConnectionEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnectionEvent) ProtoMessage() {}

func (x *ConnectionEvent) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[15]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConnectionEvent.ProtoReflect.Descriptor instead.
func (*ConnectionEvent) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{15}
}

func (x *ConnectionEvent) GetType() ConnectionEventType {
	if x != nil {
		return x.Type
	}
	return ConnectionEventType_CONNECTION_EVENT_NEW
}

func (x *ConnectionEvent) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *ConnectionEvent) GetConnection() *Connection {
	if x != nil {
		return x.Connection
	}
	return nil
}

func (x *ConnectionEvent) GetUplinkDelta() int64 {
	if x != nil {
		return x.UplinkDelta
	}
	return 0
}

func (x *ConnectionEvent) GetDownlinkDelta() int64 {
	if x != nil {
		return x.DownlinkDelta
	}
	return 0
}

func (x *ConnectionEvent) GetClosedAt() int64 {
	if x != nil {
		return x.ClosedAt
	}
	return 0
}

type ConnectionEvents struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Events        []*ConnectionEvent     `protobuf:"bytes,1,rep,name=events,proto3" json:"events,omitempty"`
	Reset_        bool                   `protobuf:"varint,2,opt,name=reset,proto3" json:"reset,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ConnectionEvents) Reset() {
	*x = ConnectionEvents{}
	mi := &file_daemon_started_service_proto_msgTypes[16]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConnectionEvents) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnectionEvents) ProtoMessage() {}

func (x *ConnectionEvents) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[16]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConnectionEvents.ProtoReflect.Descriptor instead.
func (*ConnectionEvents) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{16}
}

func (x *ConnectionEvents) GetEvents() []*ConnectionEvent {
	if x != nil {
		return x.Events
	}
	return nil
}

func (x *ConnectionEvents) GetReset_() bool {
	if x != nil {
		return x.Reset_
	}
	return false
}

type Connection struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Inbound       string                 `protobuf:"bytes,2,opt,name=inbound,proto3" json:"inbound,omitempty"`
	InboundType   string                 `protobuf:"bytes,3,opt,name=inboundType,proto3" json:"inboundType,omitempty"`
	IpVersion     int32                  `protobuf:"varint,4,opt,name=ipVersion,proto3" json:"ipVersion,omitempty"`
	Network       string                 `protobuf:"bytes,5,opt,name=network,proto3" json:"network,omitempty"`
	Source        string                 `protobuf:"bytes,6,opt,name=source,proto3" json:"source,omitempty"`
	Destination   string                 `protobuf:"bytes,7,opt,name=destination,proto3" json:"destination,omitempty"`
	Domain        string                 `protobuf:"bytes,8,opt,name=domain,proto3" json:"domain,omitempty"`
	Protocol      string                 `protobuf:"bytes,9,opt,name=protocol,proto3" json:"protocol,omitempty"`
	User          string                 `protobuf:"bytes,10,opt,name=user,proto3" json:"user,omitempty"`
	FromOutbound  string                 `protobuf:"bytes,11,opt,name=fromOutbound,proto3" json:"fromOutbound,omitempty"`
	CreatedAt     int64                  `protobuf:"varint,12,opt,name=createdAt,proto3" json:"createdAt,omitempty"`
	ClosedAt      int64                  `protobuf:"varint,13,opt,name=closedAt,proto3" json:"closedAt,omitempty"`
	Uplink        int64                  `protobuf:"varint,14,opt,name=uplink,proto3" json:"uplink,omitempty"`
	Downlink      int64                  `protobuf:"varint,15,opt,name=downlink,proto3" json:"downlink,omitempty"`
	UplinkTotal   int64                  `protobuf:"varint,16,opt,name=uplinkTotal,proto3" json:"uplinkTotal,omitempty"`
	DownlinkTotal int64                  `protobuf:"varint,17,opt,name=downlinkTotal,proto3" json:"downlinkTotal,omitempty"`
	Rule          string                 `protobuf:"bytes,18,opt,name=rule,proto3" json:"rule,omitempty"`
	Outbound      string                 `protobuf:"bytes,19,opt,name=outbound,proto3" json:"outbound,omitempty"`
	OutboundType  string                 `protobuf:"bytes,20,opt,name=outboundType,proto3" json:"outboundType,omitempty"`
	ChainList     []string               `protobuf:"bytes,21,rep,name=chainList,proto3" json:"chainList,omitempty"`
	ProcessInfo   *ProcessInfo           `protobuf:"bytes,22,opt,name=processInfo,proto3" json:"processInfo,omitempty"`
	// lx: SPEC 017 — transport detour tail of the final outbound (Chain omits it by
	// design). Order: final outbound → outward. Empty for outbounds without a detour.
	DetourList    []string `protobuf:"bytes,23,rep,name=detourList,proto3" json:"detourList,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Connection) Reset() {
	*x = Connection{}
	mi := &file_daemon_started_service_proto_msgTypes[17]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Connection) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Connection) ProtoMessage() {}

func (x *Connection) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[17]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Connection.ProtoReflect.Descriptor instead.
func (*Connection) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{17}
}

func (x *Connection) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Connection) GetInbound() string {
	if x != nil {
		return x.Inbound
	}
	return ""
}

func (x *Connection) GetInboundType() string {
	if x != nil {
		return x.InboundType
	}
	return ""
}

func (x *Connection) GetIpVersion() int32 {
	if x != nil {
		return x.IpVersion
	}
	return 0
}

func (x *Connection) GetNetwork() string {
	if x != nil {
		return x.Network
	}
	return ""
}

func (x *Connection) GetSource() string {
	if x != nil {
		return x.Source
	}
	return ""
}

func (x *Connection) GetDestination() string {
	if x != nil {
		return x.Destination
	}
	return ""
}

func (x *Connection) GetDomain() string {
	if x != nil {
		return x.Domain
	}
	return ""
}

func (x *Connection) GetProtocol() string {
	if x != nil {
		return x.Protocol
	}
	return ""
}

func (x *Connection) GetUser() string {
	if x != nil {
		return x.User
	}
	return ""
}

func (x *Connection) GetFromOutbound() string {
	if x != nil {
		return x.FromOutbound
	}
	return ""
}

func (x *Connection) GetCreatedAt() int64 {
	if x != nil {
		return x.CreatedAt
	}
	return 0
}

func (x *Connection) GetClosedAt() int64 {
	if x != nil {
		return x.ClosedAt
	}
	return 0
}

func (x *Connection) GetUplink() int64 {
	if x != nil {
		return x.Uplink
	}
	return 0
}

func (x *Connection) GetDownlink() int64 {
	if x != nil {
		return x.Downlink
	}
	return 0
}

func (x *Connection) GetUplinkTotal() int64 {
	if x != nil {
		return x.UplinkTotal
	}
	return 0
}

func (x *Connection) GetDownlinkTotal() int64 {
	if x != nil {
		return x.DownlinkTotal
	}
	return 0
}

func (x *Connection) GetRule() string {
	if x != nil {
		return x.Rule
	}
	return ""
}

func (x *Connection) GetOutbound() string {
	if x != nil {
		return x.Outbound
	}
	return ""
}

func (x *Connection) GetOutboundType() string {
	if x != nil {
		return x.OutboundType
	}
	return ""
}

func (x *Connection) GetChainList() []string {
	if x != nil {
		return x.ChainList
	}
	return nil
}

func (x *Connection) GetProcessInfo() *ProcessInfo {
	if x != nil {
		return x.ProcessInfo
	}
	return nil
}

func (x *Connection) GetDetourList() []string {
	if x != nil {
		return x.DetourList
	}
	return nil
}

type ProcessInfo struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ProcessId     uint32                 `protobuf:"varint,1,opt,name=processId,proto3" json:"processId,omitempty"`
	UserId        int32                  `protobuf:"varint,2,opt,name=userId,proto3" json:"userId,omitempty"`
	UserName      string                 `protobuf:"bytes,3,opt,name=userName,proto3" json:"userName,omitempty"`
	ProcessPath   string                 `protobuf:"bytes,4,opt,name=processPath,proto3" json:"processPath,omitempty"`
	PackageNames  []string               `protobuf:"bytes,5,rep,name=packageNames,proto3" json:"packageNames,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ProcessInfo) Reset() {
	*x = ProcessInfo{}
	mi := &file_daemon_started_service_proto_msgTypes[18]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ProcessInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProcessInfo) ProtoMessage() {}

func (x *ProcessInfo) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[18]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProcessInfo.ProtoReflect.Descriptor instead.
func (*ProcessInfo) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{18}
}

func (x *ProcessInfo) GetProcessId() uint32 {
	if x != nil {
		return x.ProcessId
	}
	return 0
}

func (x *ProcessInfo) GetUserId() int32 {
	if x != nil {
		return x.UserId
	}
	return 0
}

func (x *ProcessInfo) GetUserName() string {
	if x != nil {
		return x.UserName
	}
	return ""
}

func (x *ProcessInfo) GetProcessPath() string {
	if x != nil {
		return x.ProcessPath
	}
	return ""
}

func (x *ProcessInfo) GetPackageNames() []string {
	if x != nil {
		return x.PackageNames
	}
	return nil
}

type CloseConnectionRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CloseConnectionRequest) Reset() {
	*x = CloseConnectionRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[19]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CloseConnectionRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CloseConnectionRequest) ProtoMessage() {}

func (x *CloseConnectionRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[19]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CloseConnectionRequest.ProtoReflect.Descriptor instead.
func (*CloseConnectionRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{19}
}

func (x *CloseConnectionRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

type DeprecatedWarnings struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Warnings      []*DeprecatedWarning   `protobuf:"bytes,1,rep,name=warnings,proto3" json:"warnings,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DeprecatedWarnings) Reset() {
	*x = DeprecatedWarnings{}
	mi := &file_daemon_started_service_proto_msgTypes[20]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DeprecatedWarnings) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeprecatedWarnings) ProtoMessage() {}

func (x *DeprecatedWarnings) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[20]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeprecatedWarnings.ProtoReflect.Descriptor instead.
func (*DeprecatedWarnings) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{20}
}

func (x *DeprecatedWarnings) GetWarnings() []*DeprecatedWarning {
	if x != nil {
		return x.Warnings
	}
	return nil
}

type DeprecatedWarning struct {
	state             protoimpl.MessageState `protogen:"open.v1"`
	Message           string                 `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	Impending         bool                   `protobuf:"varint,2,opt,name=impending,proto3" json:"impending,omitempty"`
	MigrationLink     string                 `protobuf:"bytes,3,opt,name=migrationLink,proto3" json:"migrationLink,omitempty"`
	Description       string                 `protobuf:"bytes,4,opt,name=description,proto3" json:"description,omitempty"`
	DeprecatedVersion string                 `protobuf:"bytes,5,opt,name=deprecatedVersion,proto3" json:"deprecatedVersion,omitempty"`
	ScheduledVersion  string                 `protobuf:"bytes,6,opt,name=scheduledVersion,proto3" json:"scheduledVersion,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *DeprecatedWarning) Reset() {
	*x = DeprecatedWarning{}
	mi := &file_daemon_started_service_proto_msgTypes[21]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DeprecatedWarning) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeprecatedWarning) ProtoMessage() {}

func (x *DeprecatedWarning) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[21]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeprecatedWarning.ProtoReflect.Descriptor instead.
func (*DeprecatedWarning) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{21}
}

func (x *DeprecatedWarning) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *DeprecatedWarning) GetImpending() bool {
	if x != nil {
		return x.Impending
	}
	return false
}

func (x *DeprecatedWarning) GetMigrationLink() string {
	if x != nil {
		return x.MigrationLink
	}
	return ""
}

func (x *DeprecatedWarning) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *DeprecatedWarning) GetDeprecatedVersion() string {
	if x != nil {
		return x.DeprecatedVersion
	}
	return ""
}

func (x *DeprecatedWarning) GetScheduledVersion() string {
	if x != nil {
		return x.ScheduledVersion
	}
	return ""
}

type StartedAt struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	StartedAt     int64                  `protobuf:"varint,1,opt,name=startedAt,proto3" json:"startedAt,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *StartedAt) Reset() {
	*x = StartedAt{}
	mi := &file_daemon_started_service_proto_msgTypes[22]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *StartedAt) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StartedAt) ProtoMessage() {}

func (x *StartedAt) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[22]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StartedAt.ProtoReflect.Descriptor instead.
func (*StartedAt) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{22}
}

func (x *StartedAt) GetStartedAt() int64 {
	if x != nil {
		return x.StartedAt
	}
	return 0
}

type OutboundList struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Outbounds     []*GroupItem           `protobuf:"bytes,1,rep,name=outbounds,proto3" json:"outbounds,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OutboundList) Reset() {
	*x = OutboundList{}
	mi := &file_daemon_started_service_proto_msgTypes[23]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OutboundList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OutboundList) ProtoMessage() {}

func (x *OutboundList) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[23]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OutboundList.ProtoReflect.Descriptor instead.
func (*OutboundList) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{23}
}

func (x *OutboundList) GetOutbounds() []*GroupItem {
	if x != nil {
		return x.Outbounds
	}
	return nil
}

type NetworkQualityTestRequest struct {
	state             protoimpl.MessageState `protogen:"open.v1"`
	ConfigURL         string                 `protobuf:"bytes,1,opt,name=configURL,proto3" json:"configURL,omitempty"`
	OutboundTag       string                 `protobuf:"bytes,2,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	Serial            bool                   `protobuf:"varint,3,opt,name=serial,proto3" json:"serial,omitempty"`
	MaxRuntimeSeconds int32                  `protobuf:"varint,4,opt,name=maxRuntimeSeconds,proto3" json:"maxRuntimeSeconds,omitempty"`
	Http3             bool                   `protobuf:"varint,5,opt,name=http3,proto3" json:"http3,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *NetworkQualityTestRequest) Reset() {
	*x = NetworkQualityTestRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[24]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *NetworkQualityTestRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NetworkQualityTestRequest) ProtoMessage() {}

func (x *NetworkQualityTestRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[24]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NetworkQualityTestRequest.ProtoReflect.Descriptor instead.
func (*NetworkQualityTestRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{24}
}

func (x *NetworkQualityTestRequest) GetConfigURL() string {
	if x != nil {
		return x.ConfigURL
	}
	return ""
}

func (x *NetworkQualityTestRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

func (x *NetworkQualityTestRequest) GetSerial() bool {
	if x != nil {
		return x.Serial
	}
	return false
}

func (x *NetworkQualityTestRequest) GetMaxRuntimeSeconds() int32 {
	if x != nil {
		return x.MaxRuntimeSeconds
	}
	return 0
}

func (x *NetworkQualityTestRequest) GetHttp3() bool {
	if x != nil {
		return x.Http3
	}
	return false
}

type NetworkQualityTestProgress struct {
	state                    protoimpl.MessageState `protogen:"open.v1"`
	Phase                    int32                  `protobuf:"varint,1,opt,name=phase,proto3" json:"phase,omitempty"`
	DownloadCapacity         int64                  `protobuf:"varint,2,opt,name=downloadCapacity,proto3" json:"downloadCapacity,omitempty"`
	UploadCapacity           int64                  `protobuf:"varint,3,opt,name=uploadCapacity,proto3" json:"uploadCapacity,omitempty"`
	DownloadRPM              int32                  `protobuf:"varint,4,opt,name=downloadRPM,proto3" json:"downloadRPM,omitempty"`
	UploadRPM                int32                  `protobuf:"varint,5,opt,name=uploadRPM,proto3" json:"uploadRPM,omitempty"`
	IdleLatencyMs            int32                  `protobuf:"varint,6,opt,name=idleLatencyMs,proto3" json:"idleLatencyMs,omitempty"`
	ElapsedMs                int64                  `protobuf:"varint,7,opt,name=elapsedMs,proto3" json:"elapsedMs,omitempty"`
	IsFinal                  bool                   `protobuf:"varint,8,opt,name=isFinal,proto3" json:"isFinal,omitempty"`
	Error                    string                 `protobuf:"bytes,9,opt,name=error,proto3" json:"error,omitempty"`
	DownloadCapacityAccuracy int32                  `protobuf:"varint,10,opt,name=downloadCapacityAccuracy,proto3" json:"downloadCapacityAccuracy,omitempty"`
	UploadCapacityAccuracy   int32                  `protobuf:"varint,11,opt,name=uploadCapacityAccuracy,proto3" json:"uploadCapacityAccuracy,omitempty"`
	DownloadRPMAccuracy      int32                  `protobuf:"varint,12,opt,name=downloadRPMAccuracy,proto3" json:"downloadRPMAccuracy,omitempty"`
	UploadRPMAccuracy        int32                  `protobuf:"varint,13,opt,name=uploadRPMAccuracy,proto3" json:"uploadRPMAccuracy,omitempty"`
	unknownFields            protoimpl.UnknownFields
	sizeCache                protoimpl.SizeCache
}

func (x *NetworkQualityTestProgress) Reset() {
	*x = NetworkQualityTestProgress{}
	mi := &file_daemon_started_service_proto_msgTypes[25]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *NetworkQualityTestProgress) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NetworkQualityTestProgress) ProtoMessage() {}

func (x *NetworkQualityTestProgress) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[25]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NetworkQualityTestProgress.ProtoReflect.Descriptor instead.
func (*NetworkQualityTestProgress) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{25}
}

func (x *NetworkQualityTestProgress) GetPhase() int32 {
	if x != nil {
		return x.Phase
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetDownloadCapacity() int64 {
	if x != nil {
		return x.DownloadCapacity
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetUploadCapacity() int64 {
	if x != nil {
		return x.UploadCapacity
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetDownloadRPM() int32 {
	if x != nil {
		return x.DownloadRPM
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetUploadRPM() int32 {
	if x != nil {
		return x.UploadRPM
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetIdleLatencyMs() int32 {
	if x != nil {
		return x.IdleLatencyMs
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetElapsedMs() int64 {
	if x != nil {
		return x.ElapsedMs
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetIsFinal() bool {
	if x != nil {
		return x.IsFinal
	}
	return false
}

func (x *NetworkQualityTestProgress) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *NetworkQualityTestProgress) GetDownloadCapacityAccuracy() int32 {
	if x != nil {
		return x.DownloadCapacityAccuracy
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetUploadCapacityAccuracy() int32 {
	if x != nil {
		return x.UploadCapacityAccuracy
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetDownloadRPMAccuracy() int32 {
	if x != nil {
		return x.DownloadRPMAccuracy
	}
	return 0
}

func (x *NetworkQualityTestProgress) GetUploadRPMAccuracy() int32 {
	if x != nil {
		return x.UploadRPMAccuracy
	}
	return 0
}

type STUNTestRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Server        string                 `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
	OutboundTag   string                 `protobuf:"bytes,2,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *STUNTestRequest) Reset() {
	*x = STUNTestRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[26]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *STUNTestRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*STUNTestRequest) ProtoMessage() {}

func (x *STUNTestRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[26]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use STUNTestRequest.ProtoReflect.Descriptor instead.
func (*STUNTestRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{26}
}

func (x *STUNTestRequest) GetServer() string {
	if x != nil {
		return x.Server
	}
	return ""
}

func (x *STUNTestRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

type STUNTestProgress struct {
	state            protoimpl.MessageState `protogen:"open.v1"`
	Phase            int32                  `protobuf:"varint,1,opt,name=phase,proto3" json:"phase,omitempty"`
	ExternalAddr     string                 `protobuf:"bytes,2,opt,name=externalAddr,proto3" json:"externalAddr,omitempty"`
	LatencyMs        int32                  `protobuf:"varint,3,opt,name=latencyMs,proto3" json:"latencyMs,omitempty"`
	NatMapping       int32                  `protobuf:"varint,4,opt,name=natMapping,proto3" json:"natMapping,omitempty"`
	NatFiltering     int32                  `protobuf:"varint,5,opt,name=natFiltering,proto3" json:"natFiltering,omitempty"`
	IsFinal          bool                   `protobuf:"varint,6,opt,name=isFinal,proto3" json:"isFinal,omitempty"`
	Error            string                 `protobuf:"bytes,7,opt,name=error,proto3" json:"error,omitempty"`
	NatTypeSupported bool                   `protobuf:"varint,8,opt,name=natTypeSupported,proto3" json:"natTypeSupported,omitempty"`
	unknownFields    protoimpl.UnknownFields
	sizeCache        protoimpl.SizeCache
}

func (x *STUNTestProgress) Reset() {
	*x = STUNTestProgress{}
	mi := &file_daemon_started_service_proto_msgTypes[27]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *STUNTestProgress) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*STUNTestProgress) ProtoMessage() {}

func (x *STUNTestProgress) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[27]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use STUNTestProgress.ProtoReflect.Descriptor instead.
func (*STUNTestProgress) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{27}
}

func (x *STUNTestProgress) GetPhase() int32 {
	if x != nil {
		return x.Phase
	}
	return 0
}

func (x *STUNTestProgress) GetExternalAddr() string {
	if x != nil {
		return x.ExternalAddr
	}
	return ""
}

func (x *STUNTestProgress) GetLatencyMs() int32 {
	if x != nil {
		return x.LatencyMs
	}
	return 0
}

func (x *STUNTestProgress) GetNatMapping() int32 {
	if x != nil {
		return x.NatMapping
	}
	return 0
}

func (x *STUNTestProgress) GetNatFiltering() int32 {
	if x != nil {
		return x.NatFiltering
	}
	return 0
}

func (x *STUNTestProgress) GetIsFinal() bool {
	if x != nil {
		return x.IsFinal
	}
	return false
}

func (x *STUNTestProgress) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *STUNTestProgress) GetNatTypeSupported() bool {
	if x != nil {
		return x.NatTypeSupported
	}
	return false
}

type TailscaleStatusUpdate struct {
	state         protoimpl.MessageState     `protogen:"open.v1"`
	Endpoints     []*TailscaleEndpointStatus `protobuf:"bytes,1,rep,name=endpoints,proto3" json:"endpoints,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleStatusUpdate) Reset() {
	*x = TailscaleStatusUpdate{}
	mi := &file_daemon_started_service_proto_msgTypes[28]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleStatusUpdate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleStatusUpdate) ProtoMessage() {}

func (x *TailscaleStatusUpdate) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[28]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleStatusUpdate.ProtoReflect.Descriptor instead.
func (*TailscaleStatusUpdate) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{28}
}

func (x *TailscaleStatusUpdate) GetEndpoints() []*TailscaleEndpointStatus {
	if x != nil {
		return x.Endpoints
	}
	return nil
}

type TailscaleEndpointStatus struct {
	state              protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag        string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	BackendState       string                 `protobuf:"bytes,2,opt,name=backendState,proto3" json:"backendState,omitempty"`
	StateText          string                 `protobuf:"bytes,3,opt,name=stateText,proto3" json:"stateText,omitempty"`
	AuthURL            string                 `protobuf:"bytes,4,opt,name=authURL,proto3" json:"authURL,omitempty"`
	NetworkName        string                 `protobuf:"bytes,5,opt,name=networkName,proto3" json:"networkName,omitempty"`
	MagicDNSSuffix     string                 `protobuf:"bytes,6,opt,name=magicDNSSuffix,proto3" json:"magicDNSSuffix,omitempty"`
	Self               *TailscalePeer         `protobuf:"bytes,7,opt,name=self,proto3" json:"self,omitempty"`
	UserGroups         []*TailscaleUserGroup  `protobuf:"bytes,8,rep,name=userGroups,proto3" json:"userGroups,omitempty"`
	ExitNode           *TailscalePeer         `protobuf:"bytes,9,opt,name=exitNode,proto3" json:"exitNode,omitempty"`
	KeyAuth            bool                   `protobuf:"varint,10,opt,name=keyAuth,proto3" json:"keyAuth,omitempty"`
	CanShareFiles      bool                   `protobuf:"varint,11,opt,name=canShareFiles,proto3" json:"canShareFiles,omitempty"`
	WaitingFileCount   int32                  `protobuf:"varint,12,opt,name=waitingFileCount,proto3" json:"waitingFileCount,omitempty"`
	ReceivingFileCount int32                  `protobuf:"varint,13,opt,name=receivingFileCount,proto3" json:"receivingFileCount,omitempty"`
	UnreadFileCount    int32                  `protobuf:"varint,14,opt,name=unreadFileCount,proto3" json:"unreadFileCount,omitempty"`
	unknownFields      protoimpl.UnknownFields
	sizeCache          protoimpl.SizeCache
}

func (x *TailscaleEndpointStatus) Reset() {
	*x = TailscaleEndpointStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[29]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleEndpointStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleEndpointStatus) ProtoMessage() {}

func (x *TailscaleEndpointStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[29]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleEndpointStatus.ProtoReflect.Descriptor instead.
func (*TailscaleEndpointStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{29}
}

func (x *TailscaleEndpointStatus) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetBackendState() string {
	if x != nil {
		return x.BackendState
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetStateText() string {
	if x != nil {
		return x.StateText
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetAuthURL() string {
	if x != nil {
		return x.AuthURL
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetNetworkName() string {
	if x != nil {
		return x.NetworkName
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetMagicDNSSuffix() string {
	if x != nil {
		return x.MagicDNSSuffix
	}
	return ""
}

func (x *TailscaleEndpointStatus) GetSelf() *TailscalePeer {
	if x != nil {
		return x.Self
	}
	return nil
}

func (x *TailscaleEndpointStatus) GetUserGroups() []*TailscaleUserGroup {
	if x != nil {
		return x.UserGroups
	}
	return nil
}

func (x *TailscaleEndpointStatus) GetExitNode() *TailscalePeer {
	if x != nil {
		return x.ExitNode
	}
	return nil
}

func (x *TailscaleEndpointStatus) GetKeyAuth() bool {
	if x != nil {
		return x.KeyAuth
	}
	return false
}

func (x *TailscaleEndpointStatus) GetCanShareFiles() bool {
	if x != nil {
		return x.CanShareFiles
	}
	return false
}

func (x *TailscaleEndpointStatus) GetWaitingFileCount() int32 {
	if x != nil {
		return x.WaitingFileCount
	}
	return 0
}

func (x *TailscaleEndpointStatus) GetReceivingFileCount() int32 {
	if x != nil {
		return x.ReceivingFileCount
	}
	return 0
}

func (x *TailscaleEndpointStatus) GetUnreadFileCount() int32 {
	if x != nil {
		return x.UnreadFileCount
	}
	return 0
}

type TailscaleUserGroup struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UserID        int64                  `protobuf:"varint,1,opt,name=userID,proto3" json:"userID,omitempty"`
	LoginName     string                 `protobuf:"bytes,2,opt,name=loginName,proto3" json:"loginName,omitempty"`
	DisplayName   string                 `protobuf:"bytes,3,opt,name=displayName,proto3" json:"displayName,omitempty"`
	ProfilePicURL string                 `protobuf:"bytes,4,opt,name=profilePicURL,proto3" json:"profilePicURL,omitempty"`
	Peers         []*TailscalePeer       `protobuf:"bytes,5,rep,name=peers,proto3" json:"peers,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleUserGroup) Reset() {
	*x = TailscaleUserGroup{}
	mi := &file_daemon_started_service_proto_msgTypes[30]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleUserGroup) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleUserGroup) ProtoMessage() {}

func (x *TailscaleUserGroup) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[30]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleUserGroup.ProtoReflect.Descriptor instead.
func (*TailscaleUserGroup) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{30}
}

func (x *TailscaleUserGroup) GetUserID() int64 {
	if x != nil {
		return x.UserID
	}
	return 0
}

func (x *TailscaleUserGroup) GetLoginName() string {
	if x != nil {
		return x.LoginName
	}
	return ""
}

func (x *TailscaleUserGroup) GetDisplayName() string {
	if x != nil {
		return x.DisplayName
	}
	return ""
}

func (x *TailscaleUserGroup) GetProfilePicURL() string {
	if x != nil {
		return x.ProfilePicURL
	}
	return ""
}

func (x *TailscaleUserGroup) GetPeers() []*TailscalePeer {
	if x != nil {
		return x.Peers
	}
	return nil
}

type TailscalePeer struct {
	state           protoimpl.MessageState `protogen:"open.v1"`
	HostName        string                 `protobuf:"bytes,1,opt,name=hostName,proto3" json:"hostName,omitempty"`
	DnsName         string                 `protobuf:"bytes,2,opt,name=dnsName,proto3" json:"dnsName,omitempty"`
	Os              string                 `protobuf:"bytes,3,opt,name=os,proto3" json:"os,omitempty"`
	TailscaleIPs    []string               `protobuf:"bytes,4,rep,name=tailscaleIPs,proto3" json:"tailscaleIPs,omitempty"`
	Online          bool                   `protobuf:"varint,5,opt,name=online,proto3" json:"online,omitempty"`
	ExitNode        bool                   `protobuf:"varint,6,opt,name=exitNode,proto3" json:"exitNode,omitempty"`
	ExitNodeOption  bool                   `protobuf:"varint,7,opt,name=exitNodeOption,proto3" json:"exitNodeOption,omitempty"`
	Active          bool                   `protobuf:"varint,8,opt,name=active,proto3" json:"active,omitempty"`
	RxBytes         int64                  `protobuf:"varint,9,opt,name=rxBytes,proto3" json:"rxBytes,omitempty"`
	TxBytes         int64                  `protobuf:"varint,10,opt,name=txBytes,proto3" json:"txBytes,omitempty"`
	KeyExpiry       int64                  `protobuf:"varint,11,opt,name=keyExpiry,proto3" json:"keyExpiry,omitempty"`
	StableID        string                 `protobuf:"bytes,12,opt,name=stableID,proto3" json:"stableID,omitempty"`
	Expired         bool                   `protobuf:"varint,13,opt,name=expired,proto3" json:"expired,omitempty"`
	SshHostKeys     []string               `protobuf:"bytes,14,rep,name=sshHostKeys,proto3" json:"sshHostKeys,omitempty"`
	ShareeNode      bool                   `protobuf:"varint,15,opt,name=shareeNode,proto3" json:"shareeNode,omitempty"`
	LastSeen        int64                  `protobuf:"varint,16,opt,name=lastSeen,proto3" json:"lastSeen,omitempty"`
	CanReceiveFiles bool                   `protobuf:"varint,17,opt,name=canReceiveFiles,proto3" json:"canReceiveFiles,omitempty"`
	unknownFields   protoimpl.UnknownFields
	sizeCache       protoimpl.SizeCache
}

func (x *TailscalePeer) Reset() {
	*x = TailscalePeer{}
	mi := &file_daemon_started_service_proto_msgTypes[31]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscalePeer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscalePeer) ProtoMessage() {}

func (x *TailscalePeer) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[31]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscalePeer.ProtoReflect.Descriptor instead.
func (*TailscalePeer) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{31}
}

func (x *TailscalePeer) GetHostName() string {
	if x != nil {
		return x.HostName
	}
	return ""
}

func (x *TailscalePeer) GetDnsName() string {
	if x != nil {
		return x.DnsName
	}
	return ""
}

func (x *TailscalePeer) GetOs() string {
	if x != nil {
		return x.Os
	}
	return ""
}

func (x *TailscalePeer) GetTailscaleIPs() []string {
	if x != nil {
		return x.TailscaleIPs
	}
	return nil
}

func (x *TailscalePeer) GetOnline() bool {
	if x != nil {
		return x.Online
	}
	return false
}

func (x *TailscalePeer) GetExitNode() bool {
	if x != nil {
		return x.ExitNode
	}
	return false
}

func (x *TailscalePeer) GetExitNodeOption() bool {
	if x != nil {
		return x.ExitNodeOption
	}
	return false
}

func (x *TailscalePeer) GetActive() bool {
	if x != nil {
		return x.Active
	}
	return false
}

func (x *TailscalePeer) GetRxBytes() int64 {
	if x != nil {
		return x.RxBytes
	}
	return 0
}

func (x *TailscalePeer) GetTxBytes() int64 {
	if x != nil {
		return x.TxBytes
	}
	return 0
}

func (x *TailscalePeer) GetKeyExpiry() int64 {
	if x != nil {
		return x.KeyExpiry
	}
	return 0
}

func (x *TailscalePeer) GetStableID() string {
	if x != nil {
		return x.StableID
	}
	return ""
}

func (x *TailscalePeer) GetExpired() bool {
	if x != nil {
		return x.Expired
	}
	return false
}

func (x *TailscalePeer) GetSshHostKeys() []string {
	if x != nil {
		return x.SshHostKeys
	}
	return nil
}

func (x *TailscalePeer) GetShareeNode() bool {
	if x != nil {
		return x.ShareeNode
	}
	return false
}

func (x *TailscalePeer) GetLastSeen() int64 {
	if x != nil {
		return x.LastSeen
	}
	return 0
}

func (x *TailscalePeer) GetCanReceiveFiles() bool {
	if x != nil {
		return x.CanReceiveFiles
	}
	return false
}

type TailscalePingRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	PeerIP        string                 `protobuf:"bytes,2,opt,name=peerIP,proto3" json:"peerIP,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscalePingRequest) Reset() {
	*x = TailscalePingRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[32]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscalePingRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscalePingRequest) ProtoMessage() {}

func (x *TailscalePingRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[32]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscalePingRequest.ProtoReflect.Descriptor instead.
func (*TailscalePingRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{32}
}

func (x *TailscalePingRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *TailscalePingRequest) GetPeerIP() string {
	if x != nil {
		return x.PeerIP
	}
	return ""
}

type TailscalePingResponse struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	LatencyMs      float64                `protobuf:"fixed64,1,opt,name=latencyMs,proto3" json:"latencyMs,omitempty"`
	IsDirect       bool                   `protobuf:"varint,2,opt,name=isDirect,proto3" json:"isDirect,omitempty"`
	Endpoint       string                 `protobuf:"bytes,3,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
	DerpRegionID   int32                  `protobuf:"varint,4,opt,name=derpRegionID,proto3" json:"derpRegionID,omitempty"`
	DerpRegionCode string                 `protobuf:"bytes,5,opt,name=derpRegionCode,proto3" json:"derpRegionCode,omitempty"`
	Error          string                 `protobuf:"bytes,6,opt,name=error,proto3" json:"error,omitempty"`
	PeerRelay      string                 `protobuf:"bytes,7,opt,name=peerRelay,proto3" json:"peerRelay,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *TailscalePingResponse) Reset() {
	*x = TailscalePingResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[33]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscalePingResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscalePingResponse) ProtoMessage() {}

func (x *TailscalePingResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[33]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscalePingResponse.ProtoReflect.Descriptor instead.
func (*TailscalePingResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{33}
}

func (x *TailscalePingResponse) GetLatencyMs() float64 {
	if x != nil {
		return x.LatencyMs
	}
	return 0
}

func (x *TailscalePingResponse) GetIsDirect() bool {
	if x != nil {
		return x.IsDirect
	}
	return false
}

func (x *TailscalePingResponse) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

func (x *TailscalePingResponse) GetDerpRegionID() int32 {
	if x != nil {
		return x.DerpRegionID
	}
	return 0
}

func (x *TailscalePingResponse) GetDerpRegionCode() string {
	if x != nil {
		return x.DerpRegionCode
	}
	return ""
}

func (x *TailscalePingResponse) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *TailscalePingResponse) GetPeerRelay() string {
	if x != nil {
		return x.PeerRelay
	}
	return ""
}

type SetTailscaleExitNodeRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	StableID      string                 `protobuf:"bytes,2,opt,name=stableID,proto3" json:"stableID,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetTailscaleExitNodeRequest) Reset() {
	*x = SetTailscaleExitNodeRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[34]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetTailscaleExitNodeRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetTailscaleExitNodeRequest) ProtoMessage() {}

func (x *SetTailscaleExitNodeRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[34]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetTailscaleExitNodeRequest.ProtoReflect.Descriptor instead.
func (*SetTailscaleExitNodeRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{34}
}

func (x *SetTailscaleExitNodeRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *SetTailscaleExitNodeRequest) GetStableID() string {
	if x != nil {
		return x.StableID
	}
	return ""
}

type TailscaleLogoutRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleLogoutRequest) Reset() {
	*x = TailscaleLogoutRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[35]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleLogoutRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleLogoutRequest) ProtoMessage() {}

func (x *TailscaleLogoutRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[35]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleLogoutRequest.ProtoReflect.Descriptor instead.
func (*TailscaleLogoutRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{35}
}

func (x *TailscaleLogoutRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

type TailscaleSSHClientMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*TailscaleSSHClientMessage_Start
	//	*TailscaleSSHClientMessage_Input
	//	*TailscaleSSHClientMessage_Resize
	Message       isTailscaleSSHClientMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHClientMessage) Reset() {
	*x = TailscaleSSHClientMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[36]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHClientMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHClientMessage) ProtoMessage() {}

func (x *TailscaleSSHClientMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[36]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHClientMessage.ProtoReflect.Descriptor instead.
func (*TailscaleSSHClientMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{36}
}

func (x *TailscaleSSHClientMessage) GetMessage() isTailscaleSSHClientMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *TailscaleSSHClientMessage) GetStart() *TailscaleSSHStart {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHClientMessage_Start); ok {
			return x.Start
		}
	}
	return nil
}

func (x *TailscaleSSHClientMessage) GetInput() *TailscaleSSHInput {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHClientMessage_Input); ok {
			return x.Input
		}
	}
	return nil
}

func (x *TailscaleSSHClientMessage) GetResize() *TailscaleSSHResize {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHClientMessage_Resize); ok {
			return x.Resize
		}
	}
	return nil
}

type isTailscaleSSHClientMessage_Message interface {
	isTailscaleSSHClientMessage_Message()
}

type TailscaleSSHClientMessage_Start struct {
	Start *TailscaleSSHStart `protobuf:"bytes,1,opt,name=start,proto3,oneof"`
}

type TailscaleSSHClientMessage_Input struct {
	Input *TailscaleSSHInput `protobuf:"bytes,2,opt,name=input,proto3,oneof"`
}

type TailscaleSSHClientMessage_Resize struct {
	Resize *TailscaleSSHResize `protobuf:"bytes,3,opt,name=resize,proto3,oneof"`
}

func (*TailscaleSSHClientMessage_Start) isTailscaleSSHClientMessage_Message() {}

func (*TailscaleSSHClientMessage_Input) isTailscaleSSHClientMessage_Message() {}

func (*TailscaleSSHClientMessage_Resize) isTailscaleSSHClientMessage_Message() {}

type TailscaleSSHStart struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	PeerAddress   string                 `protobuf:"bytes,2,opt,name=peerAddress,proto3" json:"peerAddress,omitempty"`
	Username      string                 `protobuf:"bytes,3,opt,name=username,proto3" json:"username,omitempty"`
	TerminalType  string                 `protobuf:"bytes,4,opt,name=terminalType,proto3" json:"terminalType,omitempty"`
	Columns       int32                  `protobuf:"varint,5,opt,name=columns,proto3" json:"columns,omitempty"`
	Rows          int32                  `protobuf:"varint,6,opt,name=rows,proto3" json:"rows,omitempty"`
	WidthPixels   int32                  `protobuf:"varint,7,opt,name=widthPixels,proto3" json:"widthPixels,omitempty"`
	HeightPixels  int32                  `protobuf:"varint,8,opt,name=heightPixels,proto3" json:"heightPixels,omitempty"`
	HostKeys      []string               `protobuf:"bytes,9,rep,name=hostKeys,proto3" json:"hostKeys,omitempty"`
	ForwardAgent  bool                   `protobuf:"varint,10,opt,name=forward_agent,json=forwardAgent,proto3" json:"forward_agent,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHStart) Reset() {
	*x = TailscaleSSHStart{}
	mi := &file_daemon_started_service_proto_msgTypes[37]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHStart) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHStart) ProtoMessage() {}

func (x *TailscaleSSHStart) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[37]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHStart.ProtoReflect.Descriptor instead.
func (*TailscaleSSHStart) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{37}
}

func (x *TailscaleSSHStart) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *TailscaleSSHStart) GetPeerAddress() string {
	if x != nil {
		return x.PeerAddress
	}
	return ""
}

func (x *TailscaleSSHStart) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *TailscaleSSHStart) GetTerminalType() string {
	if x != nil {
		return x.TerminalType
	}
	return ""
}

func (x *TailscaleSSHStart) GetColumns() int32 {
	if x != nil {
		return x.Columns
	}
	return 0
}

func (x *TailscaleSSHStart) GetRows() int32 {
	if x != nil {
		return x.Rows
	}
	return 0
}

func (x *TailscaleSSHStart) GetWidthPixels() int32 {
	if x != nil {
		return x.WidthPixels
	}
	return 0
}

func (x *TailscaleSSHStart) GetHeightPixels() int32 {
	if x != nil {
		return x.HeightPixels
	}
	return 0
}

func (x *TailscaleSSHStart) GetHostKeys() []string {
	if x != nil {
		return x.HostKeys
	}
	return nil
}

func (x *TailscaleSSHStart) GetForwardAgent() bool {
	if x != nil {
		return x.ForwardAgent
	}
	return false
}

type TailscaleSSHInput struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Data          []byte                 `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHInput) Reset() {
	*x = TailscaleSSHInput{}
	mi := &file_daemon_started_service_proto_msgTypes[38]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHInput) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHInput) ProtoMessage() {}

func (x *TailscaleSSHInput) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[38]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHInput.ProtoReflect.Descriptor instead.
func (*TailscaleSSHInput) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{38}
}

func (x *TailscaleSSHInput) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type TailscaleSSHResize struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Columns       int32                  `protobuf:"varint,1,opt,name=columns,proto3" json:"columns,omitempty"`
	Rows          int32                  `protobuf:"varint,2,opt,name=rows,proto3" json:"rows,omitempty"`
	WidthPixels   int32                  `protobuf:"varint,3,opt,name=widthPixels,proto3" json:"widthPixels,omitempty"`
	HeightPixels  int32                  `protobuf:"varint,4,opt,name=heightPixels,proto3" json:"heightPixels,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHResize) Reset() {
	*x = TailscaleSSHResize{}
	mi := &file_daemon_started_service_proto_msgTypes[39]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHResize) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHResize) ProtoMessage() {}

func (x *TailscaleSSHResize) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[39]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHResize.ProtoReflect.Descriptor instead.
func (*TailscaleSSHResize) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{39}
}

func (x *TailscaleSSHResize) GetColumns() int32 {
	if x != nil {
		return x.Columns
	}
	return 0
}

func (x *TailscaleSSHResize) GetRows() int32 {
	if x != nil {
		return x.Rows
	}
	return 0
}

func (x *TailscaleSSHResize) GetWidthPixels() int32 {
	if x != nil {
		return x.WidthPixels
	}
	return 0
}

func (x *TailscaleSSHResize) GetHeightPixels() int32 {
	if x != nil {
		return x.HeightPixels
	}
	return 0
}

type TailscaleSSHServerMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*TailscaleSSHServerMessage_AuthBanner
	//	*TailscaleSSHServerMessage_Ready
	//	*TailscaleSSHServerMessage_Output
	//	*TailscaleSSHServerMessage_Exit
	//	*TailscaleSSHServerMessage_Error
	Message       isTailscaleSSHServerMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHServerMessage) Reset() {
	*x = TailscaleSSHServerMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[40]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHServerMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHServerMessage) ProtoMessage() {}

func (x *TailscaleSSHServerMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[40]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHServerMessage.ProtoReflect.Descriptor instead.
func (*TailscaleSSHServerMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{40}
}

func (x *TailscaleSSHServerMessage) GetMessage() isTailscaleSSHServerMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *TailscaleSSHServerMessage) GetAuthBanner() *TailscaleSSHAuthBanner {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHServerMessage_AuthBanner); ok {
			return x.AuthBanner
		}
	}
	return nil
}

func (x *TailscaleSSHServerMessage) GetReady() *TailscaleSSHReady {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHServerMessage_Ready); ok {
			return x.Ready
		}
	}
	return nil
}

func (x *TailscaleSSHServerMessage) GetOutput() *TailscaleSSHOutput {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHServerMessage_Output); ok {
			return x.Output
		}
	}
	return nil
}

func (x *TailscaleSSHServerMessage) GetExit() *TailscaleSSHExit {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHServerMessage_Exit); ok {
			return x.Exit
		}
	}
	return nil
}

func (x *TailscaleSSHServerMessage) GetError() *TailscaleSSHError {
	if x != nil {
		if x, ok := x.Message.(*TailscaleSSHServerMessage_Error); ok {
			return x.Error
		}
	}
	return nil
}

type isTailscaleSSHServerMessage_Message interface {
	isTailscaleSSHServerMessage_Message()
}

type TailscaleSSHServerMessage_AuthBanner struct {
	AuthBanner *TailscaleSSHAuthBanner `protobuf:"bytes,1,opt,name=authBanner,proto3,oneof"`
}

type TailscaleSSHServerMessage_Ready struct {
	Ready *TailscaleSSHReady `protobuf:"bytes,2,opt,name=ready,proto3,oneof"`
}

type TailscaleSSHServerMessage_Output struct {
	Output *TailscaleSSHOutput `protobuf:"bytes,3,opt,name=output,proto3,oneof"`
}

type TailscaleSSHServerMessage_Exit struct {
	Exit *TailscaleSSHExit `protobuf:"bytes,4,opt,name=exit,proto3,oneof"`
}

type TailscaleSSHServerMessage_Error struct {
	Error *TailscaleSSHError `protobuf:"bytes,5,opt,name=error,proto3,oneof"`
}

func (*TailscaleSSHServerMessage_AuthBanner) isTailscaleSSHServerMessage_Message() {}

func (*TailscaleSSHServerMessage_Ready) isTailscaleSSHServerMessage_Message() {}

func (*TailscaleSSHServerMessage_Output) isTailscaleSSHServerMessage_Message() {}

func (*TailscaleSSHServerMessage_Exit) isTailscaleSSHServerMessage_Message() {}

func (*TailscaleSSHServerMessage_Error) isTailscaleSSHServerMessage_Message() {}

type TailscaleSSHAuthBanner struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Message       string                 `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHAuthBanner) Reset() {
	*x = TailscaleSSHAuthBanner{}
	mi := &file_daemon_started_service_proto_msgTypes[41]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHAuthBanner) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHAuthBanner) ProtoMessage() {}

func (x *TailscaleSSHAuthBanner) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[41]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHAuthBanner.ProtoReflect.Descriptor instead.
func (*TailscaleSSHAuthBanner) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{41}
}

func (x *TailscaleSSHAuthBanner) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type TailscaleSSHReady struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHReady) Reset() {
	*x = TailscaleSSHReady{}
	mi := &file_daemon_started_service_proto_msgTypes[42]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHReady) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHReady) ProtoMessage() {}

func (x *TailscaleSSHReady) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[42]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHReady.ProtoReflect.Descriptor instead.
func (*TailscaleSSHReady) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{42}
}

type TailscaleSSHOutput struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Data          []byte                 `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHOutput) Reset() {
	*x = TailscaleSSHOutput{}
	mi := &file_daemon_started_service_proto_msgTypes[43]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHOutput) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHOutput) ProtoMessage() {}

func (x *TailscaleSSHOutput) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[43]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHOutput.ProtoReflect.Descriptor instead.
func (*TailscaleSSHOutput) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{43}
}

func (x *TailscaleSSHOutput) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type TailscaleSSHExit struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ExitCode      int32                  `protobuf:"varint,1,opt,name=exitCode,proto3" json:"exitCode,omitempty"`
	Signal        string                 `protobuf:"bytes,2,opt,name=signal,proto3" json:"signal,omitempty"`
	ErrorMessage  string                 `protobuf:"bytes,3,opt,name=errorMessage,proto3" json:"errorMessage,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHExit) Reset() {
	*x = TailscaleSSHExit{}
	mi := &file_daemon_started_service_proto_msgTypes[44]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHExit) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHExit) ProtoMessage() {}

func (x *TailscaleSSHExit) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[44]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHExit.ProtoReflect.Descriptor instead.
func (*TailscaleSSHExit) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{44}
}

func (x *TailscaleSSHExit) GetExitCode() int32 {
	if x != nil {
		return x.ExitCode
	}
	return 0
}

func (x *TailscaleSSHExit) GetSignal() string {
	if x != nil {
		return x.Signal
	}
	return ""
}

func (x *TailscaleSSHExit) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

type TailscaleSSHError struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Message       string                 `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TailscaleSSHError) Reset() {
	*x = TailscaleSSHError{}
	mi := &file_daemon_started_service_proto_msgTypes[45]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TailscaleSSHError) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TailscaleSSHError) ProtoMessage() {}

func (x *TailscaleSSHError) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[45]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TailscaleSSHError.ProtoReflect.Descriptor instead.
func (*TailscaleSSHError) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{45}
}

func (x *TailscaleSSHError) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type SubscribeTaildropInboxRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubscribeTaildropInboxRequest) Reset() {
	*x = SubscribeTaildropInboxRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[46]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubscribeTaildropInboxRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubscribeTaildropInboxRequest) ProtoMessage() {}

func (x *SubscribeTaildropInboxRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[46]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubscribeTaildropInboxRequest.ProtoReflect.Descriptor instead.
func (*SubscribeTaildropInboxRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{46}
}

func (x *SubscribeTaildropInboxRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

type MarkTaildropInboxReadRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *MarkTaildropInboxReadRequest) Reset() {
	*x = MarkTaildropInboxReadRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[47]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *MarkTaildropInboxReadRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MarkTaildropInboxReadRequest) ProtoMessage() {}

func (x *MarkTaildropInboxReadRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[47]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MarkTaildropInboxReadRequest.ProtoReflect.Descriptor instead.
func (*MarkTaildropInboxReadRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{47}
}

func (x *MarkTaildropInboxReadRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

type TaildropInbox struct {
	state         protoimpl.MessageState   `protogen:"open.v1"`
	EndpointTag   string                   `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	Files         []*TaildropFile          `protobuf:"bytes,2,rep,name=files,proto3" json:"files,omitempty"`
	Receiving     []*TaildropReceivingFile `protobuf:"bytes,3,rep,name=receiving,proto3" json:"receiving,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropInbox) Reset() {
	*x = TaildropInbox{}
	mi := &file_daemon_started_service_proto_msgTypes[48]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropInbox) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropInbox) ProtoMessage() {}

func (x *TaildropInbox) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[48]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropInbox.ProtoReflect.Descriptor instead.
func (*TaildropInbox) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{48}
}

func (x *TaildropInbox) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *TaildropInbox) GetFiles() []*TaildropFile {
	if x != nil {
		return x.Files
	}
	return nil
}

func (x *TaildropInbox) GetReceiving() []*TaildropReceivingFile {
	if x != nil {
		return x.Receiving
	}
	return nil
}

type TaildropFile struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Size          int64                  `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	SenderName    string                 `protobuf:"bytes,3,opt,name=senderName,proto3" json:"senderName,omitempty"`
	ModifiedAt    int64                  `protobuf:"varint,4,opt,name=modifiedAt,proto3" json:"modifiedAt,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropFile) Reset() {
	*x = TaildropFile{}
	mi := &file_daemon_started_service_proto_msgTypes[49]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropFile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropFile) ProtoMessage() {}

func (x *TaildropFile) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[49]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropFile.ProtoReflect.Descriptor instead.
func (*TaildropFile) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{49}
}

func (x *TaildropFile) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *TaildropFile) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *TaildropFile) GetSenderName() string {
	if x != nil {
		return x.SenderName
	}
	return ""
}

func (x *TaildropFile) GetModifiedAt() int64 {
	if x != nil {
		return x.ModifiedAt
	}
	return 0
}

type TaildropReceivingFile struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Size          int64                  `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	ReceivedBytes int64                  `protobuf:"varint,3,opt,name=receivedBytes,proto3" json:"receivedBytes,omitempty"`
	SenderID      string                 `protobuf:"bytes,4,opt,name=senderID,proto3" json:"senderID,omitempty"`
	SenderName    string                 `protobuf:"bytes,5,opt,name=senderName,proto3" json:"senderName,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropReceivingFile) Reset() {
	*x = TaildropReceivingFile{}
	mi := &file_daemon_started_service_proto_msgTypes[50]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropReceivingFile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropReceivingFile) ProtoMessage() {}

func (x *TaildropReceivingFile) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[50]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropReceivingFile.ProtoReflect.Descriptor instead.
func (*TaildropReceivingFile) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{50}
}

func (x *TaildropReceivingFile) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *TaildropReceivingFile) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *TaildropReceivingFile) GetReceivedBytes() int64 {
	if x != nil {
		return x.ReceivedBytes
	}
	return 0
}

func (x *TaildropReceivingFile) GetSenderID() string {
	if x != nil {
		return x.SenderID
	}
	return ""
}

func (x *TaildropReceivingFile) GetSenderName() string {
	if x != nil {
		return x.SenderName
	}
	return ""
}

type TaildropSendClientMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*TaildropSendClientMessage_Start
	//	*TaildropSendClientMessage_Chunk
	//	*TaildropSendClientMessage_FileDone
	Message       isTaildropSendClientMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropSendClientMessage) Reset() {
	*x = TaildropSendClientMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[51]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropSendClientMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropSendClientMessage) ProtoMessage() {}

func (x *TaildropSendClientMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[51]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropSendClientMessage.ProtoReflect.Descriptor instead.
func (*TaildropSendClientMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{51}
}

func (x *TaildropSendClientMessage) GetMessage() isTaildropSendClientMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *TaildropSendClientMessage) GetStart() *TaildropSendStart {
	if x != nil {
		if x, ok := x.Message.(*TaildropSendClientMessage_Start); ok {
			return x.Start
		}
	}
	return nil
}

func (x *TaildropSendClientMessage) GetChunk() *TaildropFileChunk {
	if x != nil {
		if x, ok := x.Message.(*TaildropSendClientMessage_Chunk); ok {
			return x.Chunk
		}
	}
	return nil
}

func (x *TaildropSendClientMessage) GetFileDone() *TaildropFileDone {
	if x != nil {
		if x, ok := x.Message.(*TaildropSendClientMessage_FileDone); ok {
			return x.FileDone
		}
	}
	return nil
}

type isTaildropSendClientMessage_Message interface {
	isTaildropSendClientMessage_Message()
}

type TaildropSendClientMessage_Start struct {
	Start *TaildropSendStart `protobuf:"bytes,1,opt,name=start,proto3,oneof"`
}

type TaildropSendClientMessage_Chunk struct {
	Chunk *TaildropFileChunk `protobuf:"bytes,2,opt,name=chunk,proto3,oneof"`
}

type TaildropSendClientMessage_FileDone struct {
	FileDone *TaildropFileDone `protobuf:"bytes,3,opt,name=fileDone,proto3,oneof"`
}

func (*TaildropSendClientMessage_Start) isTaildropSendClientMessage_Message() {}

func (*TaildropSendClientMessage_Chunk) isTaildropSendClientMessage_Message() {}

func (*TaildropSendClientMessage_FileDone) isTaildropSendClientMessage_Message() {}

type TaildropSendStart struct {
	state         protoimpl.MessageState  `protogen:"open.v1"`
	EndpointTag   string                  `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	PeerStableID  string                  `protobuf:"bytes,2,opt,name=peerStableID,proto3" json:"peerStableID,omitempty"`
	Files         []*TaildropOutgoingFile `protobuf:"bytes,3,rep,name=files,proto3" json:"files,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropSendStart) Reset() {
	*x = TaildropSendStart{}
	mi := &file_daemon_started_service_proto_msgTypes[52]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropSendStart) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropSendStart) ProtoMessage() {}

func (x *TaildropSendStart) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[52]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropSendStart.ProtoReflect.Descriptor instead.
func (*TaildropSendStart) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{52}
}

func (x *TaildropSendStart) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *TaildropSendStart) GetPeerStableID() string {
	if x != nil {
		return x.PeerStableID
	}
	return ""
}

func (x *TaildropSendStart) GetFiles() []*TaildropOutgoingFile {
	if x != nil {
		return x.Files
	}
	return nil
}

type TaildropOutgoingFile struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Size          int64                  `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropOutgoingFile) Reset() {
	*x = TaildropOutgoingFile{}
	mi := &file_daemon_started_service_proto_msgTypes[53]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropOutgoingFile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropOutgoingFile) ProtoMessage() {}

func (x *TaildropOutgoingFile) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[53]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropOutgoingFile.ProtoReflect.Descriptor instead.
func (*TaildropOutgoingFile) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{53}
}

func (x *TaildropOutgoingFile) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *TaildropOutgoingFile) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

type TaildropFileChunk struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Data          []byte                 `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropFileChunk) Reset() {
	*x = TaildropFileChunk{}
	mi := &file_daemon_started_service_proto_msgTypes[54]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropFileChunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropFileChunk) ProtoMessage() {}

func (x *TaildropFileChunk) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[54]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropFileChunk.ProtoReflect.Descriptor instead.
func (*TaildropFileChunk) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{54}
}

func (x *TaildropFileChunk) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type TaildropFileDone struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropFileDone) Reset() {
	*x = TaildropFileDone{}
	mi := &file_daemon_started_service_proto_msgTypes[55]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropFileDone) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropFileDone) ProtoMessage() {}

func (x *TaildropFileDone) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[55]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropFileDone.ProtoReflect.Descriptor instead.
func (*TaildropFileDone) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{55}
}

type TaildropSendServerMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*TaildropSendServerMessage_Progress
	//	*TaildropSendServerMessage_ReceivedBytes
	Message       isTaildropSendServerMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropSendServerMessage) Reset() {
	*x = TaildropSendServerMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[56]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropSendServerMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropSendServerMessage) ProtoMessage() {}

func (x *TaildropSendServerMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[56]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropSendServerMessage.ProtoReflect.Descriptor instead.
func (*TaildropSendServerMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{56}
}

func (x *TaildropSendServerMessage) GetMessage() isTaildropSendServerMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *TaildropSendServerMessage) GetProgress() *TaildropSendProgress {
	if x != nil {
		if x, ok := x.Message.(*TaildropSendServerMessage_Progress); ok {
			return x.Progress
		}
	}
	return nil
}

func (x *TaildropSendServerMessage) GetReceivedBytes() int64 {
	if x != nil {
		if x, ok := x.Message.(*TaildropSendServerMessage_ReceivedBytes); ok {
			return x.ReceivedBytes
		}
	}
	return 0
}

type isTaildropSendServerMessage_Message interface {
	isTaildropSendServerMessage_Message()
}

type TaildropSendServerMessage_Progress struct {
	Progress *TaildropSendProgress `protobuf:"bytes,1,opt,name=progress,proto3,oneof"`
}

type TaildropSendServerMessage_ReceivedBytes struct {
	ReceivedBytes int64 `protobuf:"varint,2,opt,name=receivedBytes,proto3,oneof"`
}

func (*TaildropSendServerMessage_Progress) isTaildropSendServerMessage_Message() {}

func (*TaildropSendServerMessage_ReceivedBytes) isTaildropSendServerMessage_Message() {}

type TaildropSendProgress struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	FileIndex     int32                  `protobuf:"varint,1,opt,name=fileIndex,proto3" json:"fileIndex,omitempty"`
	SentBytes     int64                  `protobuf:"varint,2,opt,name=sentBytes,proto3" json:"sentBytes,omitempty"`
	FileCompleted bool                   `protobuf:"varint,3,opt,name=fileCompleted,proto3" json:"fileCompleted,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TaildropSendProgress) Reset() {
	*x = TaildropSendProgress{}
	mi := &file_daemon_started_service_proto_msgTypes[57]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TaildropSendProgress) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TaildropSendProgress) ProtoMessage() {}

func (x *TaildropSendProgress) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[57]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TaildropSendProgress.ProtoReflect.Descriptor instead.
func (*TaildropSendProgress) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{57}
}

func (x *TaildropSendProgress) GetFileIndex() int32 {
	if x != nil {
		return x.FileIndex
	}
	return 0
}

func (x *TaildropSendProgress) GetSentBytes() int64 {
	if x != nil {
		return x.SentBytes
	}
	return 0
}

func (x *TaildropSendProgress) GetFileCompleted() bool {
	if x != nil {
		return x.FileCompleted
	}
	return false
}

type DownloadTaildropFileRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	Name          string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DownloadTaildropFileRequest) Reset() {
	*x = DownloadTaildropFileRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[58]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DownloadTaildropFileRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DownloadTaildropFileRequest) ProtoMessage() {}

func (x *DownloadTaildropFileRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[58]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DownloadTaildropFileRequest.ProtoReflect.Descriptor instead.
func (*DownloadTaildropFileRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{58}
}

func (x *DownloadTaildropFileRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *DownloadTaildropFileRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type DownloadTaildropFileChunk struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Size          int64                  `protobuf:"varint,1,opt,name=size,proto3" json:"size,omitempty"`
	Data          []byte                 `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DownloadTaildropFileChunk) Reset() {
	*x = DownloadTaildropFileChunk{}
	mi := &file_daemon_started_service_proto_msgTypes[59]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DownloadTaildropFileChunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DownloadTaildropFileChunk) ProtoMessage() {}

func (x *DownloadTaildropFileChunk) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[59]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DownloadTaildropFileChunk.ProtoReflect.Descriptor instead.
func (*DownloadTaildropFileChunk) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{59}
}

func (x *DownloadTaildropFileChunk) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *DownloadTaildropFileChunk) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type DeleteTaildropFileRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	Name          string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DeleteTaildropFileRequest) Reset() {
	*x = DeleteTaildropFileRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[60]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DeleteTaildropFileRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeleteTaildropFileRequest) ProtoMessage() {}

func (x *DeleteTaildropFileRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[60]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeleteTaildropFileRequest.ProtoReflect.Descriptor instead.
func (*DeleteTaildropFileRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{60}
}

func (x *DeleteTaildropFileRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *DeleteTaildropFileRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type CancelTaildropReceivingRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	SenderID      string                 `protobuf:"bytes,2,opt,name=senderID,proto3" json:"senderID,omitempty"`
	Name          string                 `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CancelTaildropReceivingRequest) Reset() {
	*x = CancelTaildropReceivingRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[61]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CancelTaildropReceivingRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CancelTaildropReceivingRequest) ProtoMessage() {}

func (x *CancelTaildropReceivingRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[61]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CancelTaildropReceivingRequest.ProtoReflect.Descriptor instead.
func (*CancelTaildropReceivingRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{61}
}

func (x *CancelTaildropReceivingRequest) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *CancelTaildropReceivingRequest) GetSenderID() string {
	if x != nil {
		return x.SenderID
	}
	return ""
}

func (x *CancelTaildropReceivingRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type USBProviderMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*USBProviderMessage_Attach
	//	*USBProviderMessage_Detach
	//	*USBProviderMessage_UrbResponse
	Message       isUSBProviderMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBProviderMessage) Reset() {
	*x = USBProviderMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[62]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBProviderMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBProviderMessage) ProtoMessage() {}

func (x *USBProviderMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[62]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBProviderMessage.ProtoReflect.Descriptor instead.
func (*USBProviderMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{62}
}

func (x *USBProviderMessage) GetMessage() isUSBProviderMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *USBProviderMessage) GetAttach() *USBDeviceAttach {
	if x != nil {
		if x, ok := x.Message.(*USBProviderMessage_Attach); ok {
			return x.Attach
		}
	}
	return nil
}

func (x *USBProviderMessage) GetDetach() *USBDeviceDetach {
	if x != nil {
		if x, ok := x.Message.(*USBProviderMessage_Detach); ok {
			return x.Detach
		}
	}
	return nil
}

func (x *USBProviderMessage) GetUrbResponse() *USBURBResponse {
	if x != nil {
		if x, ok := x.Message.(*USBProviderMessage_UrbResponse); ok {
			return x.UrbResponse
		}
	}
	return nil
}

type isUSBProviderMessage_Message interface {
	isUSBProviderMessage_Message()
}

type USBProviderMessage_Attach struct {
	Attach *USBDeviceAttach `protobuf:"bytes,1,opt,name=attach,proto3,oneof"`
}

type USBProviderMessage_Detach struct {
	Detach *USBDeviceDetach `protobuf:"bytes,2,opt,name=detach,proto3,oneof"`
}

type USBProviderMessage_UrbResponse struct {
	UrbResponse *USBURBResponse `protobuf:"bytes,3,opt,name=urbResponse,proto3,oneof"`
}

func (*USBProviderMessage_Attach) isUSBProviderMessage_Message() {}

func (*USBProviderMessage_Detach) isUSBProviderMessage_Message() {}

func (*USBProviderMessage_UrbResponse) isUSBProviderMessage_Message() {}

type USBServerMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Message:
	//
	//	*USBServerMessage_Ready
	//	*USBServerMessage_UrbRequest
	//	*USBServerMessage_Abort
	//	*USBServerMessage_Error
	Message       isUSBServerMessage_Message `protobuf_oneof:"message"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBServerMessage) Reset() {
	*x = USBServerMessage{}
	mi := &file_daemon_started_service_proto_msgTypes[63]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBServerMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBServerMessage) ProtoMessage() {}

func (x *USBServerMessage) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[63]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBServerMessage.ProtoReflect.Descriptor instead.
func (*USBServerMessage) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{63}
}

func (x *USBServerMessage) GetMessage() isUSBServerMessage_Message {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *USBServerMessage) GetReady() *USBDeviceReady {
	if x != nil {
		if x, ok := x.Message.(*USBServerMessage_Ready); ok {
			return x.Ready
		}
	}
	return nil
}

func (x *USBServerMessage) GetUrbRequest() *USBURBRequest {
	if x != nil {
		if x, ok := x.Message.(*USBServerMessage_UrbRequest); ok {
			return x.UrbRequest
		}
	}
	return nil
}

func (x *USBServerMessage) GetAbort() *USBEndpointAbort {
	if x != nil {
		if x, ok := x.Message.(*USBServerMessage_Abort); ok {
			return x.Abort
		}
	}
	return nil
}

func (x *USBServerMessage) GetError() *USBError {
	if x != nil {
		if x, ok := x.Message.(*USBServerMessage_Error); ok {
			return x.Error
		}
	}
	return nil
}

type isUSBServerMessage_Message interface {
	isUSBServerMessage_Message()
}

type USBServerMessage_Ready struct {
	Ready *USBDeviceReady `protobuf:"bytes,1,opt,name=ready,proto3,oneof"`
}

type USBServerMessage_UrbRequest struct {
	UrbRequest *USBURBRequest `protobuf:"bytes,2,opt,name=urbRequest,proto3,oneof"`
}

type USBServerMessage_Abort struct {
	Abort *USBEndpointAbort `protobuf:"bytes,3,opt,name=abort,proto3,oneof"`
}

type USBServerMessage_Error struct {
	Error *USBError `protobuf:"bytes,4,opt,name=error,proto3,oneof"`
}

func (*USBServerMessage_Ready) isUSBServerMessage_Message() {}

func (*USBServerMessage_UrbRequest) isUSBServerMessage_Message() {}

func (*USBServerMessage_Abort) isUSBServerMessage_Message() {}

func (*USBServerMessage_Error) isUSBServerMessage_Message() {}

type USBDeviceDescriptor struct {
	state              protoimpl.MessageState `protogen:"open.v1"`
	DeviceId           string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	BusNum             uint32                 `protobuf:"varint,2,opt,name=busNum,proto3" json:"busNum,omitempty"`
	DevNum             uint32                 `protobuf:"varint,3,opt,name=devNum,proto3" json:"devNum,omitempty"`
	Speed              uint32                 `protobuf:"varint,4,opt,name=speed,proto3" json:"speed,omitempty"`
	VendorId           uint32                 `protobuf:"varint,5,opt,name=vendorId,proto3" json:"vendorId,omitempty"`
	ProductId          uint32                 `protobuf:"varint,6,opt,name=productId,proto3" json:"productId,omitempty"`
	BcdDevice          uint32                 `protobuf:"varint,7,opt,name=bcdDevice,proto3" json:"bcdDevice,omitempty"`
	DeviceClass        uint32                 `protobuf:"varint,8,opt,name=deviceClass,proto3" json:"deviceClass,omitempty"`
	DeviceSubClass     uint32                 `protobuf:"varint,9,opt,name=deviceSubClass,proto3" json:"deviceSubClass,omitempty"`
	DeviceProtocol     uint32                 `protobuf:"varint,10,opt,name=deviceProtocol,proto3" json:"deviceProtocol,omitempty"`
	ConfigurationValue uint32                 `protobuf:"varint,11,opt,name=configurationValue,proto3" json:"configurationValue,omitempty"`
	NumConfigurations  uint32                 `protobuf:"varint,12,opt,name=numConfigurations,proto3" json:"numConfigurations,omitempty"`
	Interfaces         []*USBInterface        `protobuf:"bytes,13,rep,name=interfaces,proto3" json:"interfaces,omitempty"`
	Serial             string                 `protobuf:"bytes,14,opt,name=serial,proto3" json:"serial,omitempty"`
	Product            string                 `protobuf:"bytes,15,opt,name=product,proto3" json:"product,omitempty"`
	unknownFields      protoimpl.UnknownFields
	sizeCache          protoimpl.SizeCache
}

func (x *USBDeviceDescriptor) Reset() {
	*x = USBDeviceDescriptor{}
	mi := &file_daemon_started_service_proto_msgTypes[64]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBDeviceDescriptor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBDeviceDescriptor) ProtoMessage() {}

func (x *USBDeviceDescriptor) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[64]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBDeviceDescriptor.ProtoReflect.Descriptor instead.
func (*USBDeviceDescriptor) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{64}
}

func (x *USBDeviceDescriptor) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBDeviceDescriptor) GetBusNum() uint32 {
	if x != nil {
		return x.BusNum
	}
	return 0
}

func (x *USBDeviceDescriptor) GetDevNum() uint32 {
	if x != nil {
		return x.DevNum
	}
	return 0
}

func (x *USBDeviceDescriptor) GetSpeed() uint32 {
	if x != nil {
		return x.Speed
	}
	return 0
}

func (x *USBDeviceDescriptor) GetVendorId() uint32 {
	if x != nil {
		return x.VendorId
	}
	return 0
}

func (x *USBDeviceDescriptor) GetProductId() uint32 {
	if x != nil {
		return x.ProductId
	}
	return 0
}

func (x *USBDeviceDescriptor) GetBcdDevice() uint32 {
	if x != nil {
		return x.BcdDevice
	}
	return 0
}

func (x *USBDeviceDescriptor) GetDeviceClass() uint32 {
	if x != nil {
		return x.DeviceClass
	}
	return 0
}

func (x *USBDeviceDescriptor) GetDeviceSubClass() uint32 {
	if x != nil {
		return x.DeviceSubClass
	}
	return 0
}

func (x *USBDeviceDescriptor) GetDeviceProtocol() uint32 {
	if x != nil {
		return x.DeviceProtocol
	}
	return 0
}

func (x *USBDeviceDescriptor) GetConfigurationValue() uint32 {
	if x != nil {
		return x.ConfigurationValue
	}
	return 0
}

func (x *USBDeviceDescriptor) GetNumConfigurations() uint32 {
	if x != nil {
		return x.NumConfigurations
	}
	return 0
}

func (x *USBDeviceDescriptor) GetInterfaces() []*USBInterface {
	if x != nil {
		return x.Interfaces
	}
	return nil
}

func (x *USBDeviceDescriptor) GetSerial() string {
	if x != nil {
		return x.Serial
	}
	return ""
}

func (x *USBDeviceDescriptor) GetProduct() string {
	if x != nil {
		return x.Product
	}
	return ""
}

type USBDeviceAttach struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ServerTag     string                 `protobuf:"bytes,1,opt,name=serverTag,proto3" json:"serverTag,omitempty"`
	Descriptor_   *USBDeviceDescriptor   `protobuf:"bytes,2,opt,name=descriptor,proto3" json:"descriptor,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBDeviceAttach) Reset() {
	*x = USBDeviceAttach{}
	mi := &file_daemon_started_service_proto_msgTypes[65]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBDeviceAttach) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBDeviceAttach) ProtoMessage() {}

func (x *USBDeviceAttach) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[65]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBDeviceAttach.ProtoReflect.Descriptor instead.
func (*USBDeviceAttach) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{65}
}

func (x *USBDeviceAttach) GetServerTag() string {
	if x != nil {
		return x.ServerTag
	}
	return ""
}

func (x *USBDeviceAttach) GetDescriptor_() *USBDeviceDescriptor {
	if x != nil {
		return x.Descriptor_
	}
	return nil
}

type USBInterface struct {
	state             protoimpl.MessageState `protogen:"open.v1"`
	InterfaceClass    uint32                 `protobuf:"varint,1,opt,name=interfaceClass,proto3" json:"interfaceClass,omitempty"`
	InterfaceSubClass uint32                 `protobuf:"varint,2,opt,name=interfaceSubClass,proto3" json:"interfaceSubClass,omitempty"`
	InterfaceProtocol uint32                 `protobuf:"varint,3,opt,name=interfaceProtocol,proto3" json:"interfaceProtocol,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *USBInterface) Reset() {
	*x = USBInterface{}
	mi := &file_daemon_started_service_proto_msgTypes[66]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBInterface) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBInterface) ProtoMessage() {}

func (x *USBInterface) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[66]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBInterface.ProtoReflect.Descriptor instead.
func (*USBInterface) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{66}
}

func (x *USBInterface) GetInterfaceClass() uint32 {
	if x != nil {
		return x.InterfaceClass
	}
	return 0
}

func (x *USBInterface) GetInterfaceSubClass() uint32 {
	if x != nil {
		return x.InterfaceSubClass
	}
	return 0
}

func (x *USBInterface) GetInterfaceProtocol() uint32 {
	if x != nil {
		return x.InterfaceProtocol
	}
	return 0
}

type USBDeviceDetach struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	DeviceId      string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBDeviceDetach) Reset() {
	*x = USBDeviceDetach{}
	mi := &file_daemon_started_service_proto_msgTypes[67]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBDeviceDetach) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBDeviceDetach) ProtoMessage() {}

func (x *USBDeviceDetach) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[67]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBDeviceDetach.ProtoReflect.Descriptor instead.
func (*USBDeviceDetach) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{67}
}

func (x *USBDeviceDetach) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

type USBDeviceReady struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	DeviceId      string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	BusId         string                 `protobuf:"bytes,2,opt,name=busId,proto3" json:"busId,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBDeviceReady) Reset() {
	*x = USBDeviceReady{}
	mi := &file_daemon_started_service_proto_msgTypes[68]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBDeviceReady) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBDeviceReady) ProtoMessage() {}

func (x *USBDeviceReady) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[68]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBDeviceReady.ProtoReflect.Descriptor instead.
func (*USBDeviceReady) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{68}
}

func (x *USBDeviceReady) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBDeviceReady) GetBusId() string {
	if x != nil {
		return x.BusId
	}
	return ""
}

type USBURBRequest struct {
	state                protoimpl.MessageState `protogen:"open.v1"`
	DeviceId             string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	Seq                  uint64                 `protobuf:"varint,2,opt,name=seq,proto3" json:"seq,omitempty"`
	Endpoint             uint32                 `protobuf:"varint,3,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
	DirectionIn          bool                   `protobuf:"varint,4,opt,name=directionIn,proto3" json:"directionIn,omitempty"`
	TransferFlags        uint32                 `protobuf:"varint,5,opt,name=transferFlags,proto3" json:"transferFlags,omitempty"`
	Setup                []byte                 `protobuf:"bytes,6,opt,name=setup,proto3" json:"setup,omitempty"`
	TransferBufferLength uint32                 `protobuf:"varint,7,opt,name=transferBufferLength,proto3" json:"transferBufferLength,omitempty"`
	OutData              []byte                 `protobuf:"bytes,8,opt,name=outData,proto3" json:"outData,omitempty"`
	NumberOfPackets      int32                  `protobuf:"varint,9,opt,name=numberOfPackets,proto3" json:"numberOfPackets,omitempty"`
	StartFrame           int32                  `protobuf:"varint,10,opt,name=startFrame,proto3" json:"startFrame,omitempty"`
	Interval             int32                  `protobuf:"varint,11,opt,name=interval,proto3" json:"interval,omitempty"`
	IsoPackets           []*USBIsoPacket        `protobuf:"bytes,12,rep,name=isoPackets,proto3" json:"isoPackets,omitempty"`
	unknownFields        protoimpl.UnknownFields
	sizeCache            protoimpl.SizeCache
}

func (x *USBURBRequest) Reset() {
	*x = USBURBRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[69]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBURBRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBURBRequest) ProtoMessage() {}

func (x *USBURBRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[69]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBURBRequest.ProtoReflect.Descriptor instead.
func (*USBURBRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{69}
}

func (x *USBURBRequest) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBURBRequest) GetSeq() uint64 {
	if x != nil {
		return x.Seq
	}
	return 0
}

func (x *USBURBRequest) GetEndpoint() uint32 {
	if x != nil {
		return x.Endpoint
	}
	return 0
}

func (x *USBURBRequest) GetDirectionIn() bool {
	if x != nil {
		return x.DirectionIn
	}
	return false
}

func (x *USBURBRequest) GetTransferFlags() uint32 {
	if x != nil {
		return x.TransferFlags
	}
	return 0
}

func (x *USBURBRequest) GetSetup() []byte {
	if x != nil {
		return x.Setup
	}
	return nil
}

func (x *USBURBRequest) GetTransferBufferLength() uint32 {
	if x != nil {
		return x.TransferBufferLength
	}
	return 0
}

func (x *USBURBRequest) GetOutData() []byte {
	if x != nil {
		return x.OutData
	}
	return nil
}

func (x *USBURBRequest) GetNumberOfPackets() int32 {
	if x != nil {
		return x.NumberOfPackets
	}
	return 0
}

func (x *USBURBRequest) GetStartFrame() int32 {
	if x != nil {
		return x.StartFrame
	}
	return 0
}

func (x *USBURBRequest) GetInterval() int32 {
	if x != nil {
		return x.Interval
	}
	return 0
}

func (x *USBURBRequest) GetIsoPackets() []*USBIsoPacket {
	if x != nil {
		return x.IsoPackets
	}
	return nil
}

type USBURBResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	DeviceId      string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	Seq           uint64                 `protobuf:"varint,2,opt,name=seq,proto3" json:"seq,omitempty"`
	Status        int32                  `protobuf:"varint,3,opt,name=status,proto3" json:"status,omitempty"`
	ActualLength  int32                  `protobuf:"varint,4,opt,name=actualLength,proto3" json:"actualLength,omitempty"`
	InData        []byte                 `protobuf:"bytes,5,opt,name=inData,proto3" json:"inData,omitempty"`
	IsoPackets    []*USBIsoPacket        `protobuf:"bytes,6,rep,name=isoPackets,proto3" json:"isoPackets,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBURBResponse) Reset() {
	*x = USBURBResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[70]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBURBResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBURBResponse) ProtoMessage() {}

func (x *USBURBResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[70]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBURBResponse.ProtoReflect.Descriptor instead.
func (*USBURBResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{70}
}

func (x *USBURBResponse) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBURBResponse) GetSeq() uint64 {
	if x != nil {
		return x.Seq
	}
	return 0
}

func (x *USBURBResponse) GetStatus() int32 {
	if x != nil {
		return x.Status
	}
	return 0
}

func (x *USBURBResponse) GetActualLength() int32 {
	if x != nil {
		return x.ActualLength
	}
	return 0
}

func (x *USBURBResponse) GetInData() []byte {
	if x != nil {
		return x.InData
	}
	return nil
}

func (x *USBURBResponse) GetIsoPackets() []*USBIsoPacket {
	if x != nil {
		return x.IsoPackets
	}
	return nil
}

type USBIsoPacket struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Offset        int32                  `protobuf:"varint,1,opt,name=offset,proto3" json:"offset,omitempty"`
	Length        int32                  `protobuf:"varint,2,opt,name=length,proto3" json:"length,omitempty"`
	ActualLength  int32                  `protobuf:"varint,3,opt,name=actualLength,proto3" json:"actualLength,omitempty"`
	Status        int32                  `protobuf:"varint,4,opt,name=status,proto3" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBIsoPacket) Reset() {
	*x = USBIsoPacket{}
	mi := &file_daemon_started_service_proto_msgTypes[71]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBIsoPacket) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBIsoPacket) ProtoMessage() {}

func (x *USBIsoPacket) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[71]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBIsoPacket.ProtoReflect.Descriptor instead.
func (*USBIsoPacket) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{71}
}

func (x *USBIsoPacket) GetOffset() int32 {
	if x != nil {
		return x.Offset
	}
	return 0
}

func (x *USBIsoPacket) GetLength() int32 {
	if x != nil {
		return x.Length
	}
	return 0
}

func (x *USBIsoPacket) GetActualLength() int32 {
	if x != nil {
		return x.ActualLength
	}
	return 0
}

func (x *USBIsoPacket) GetStatus() int32 {
	if x != nil {
		return x.Status
	}
	return 0
}

type USBEndpointAbort struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	DeviceId      string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	Endpoint      uint32                 `protobuf:"varint,2,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBEndpointAbort) Reset() {
	*x = USBEndpointAbort{}
	mi := &file_daemon_started_service_proto_msgTypes[72]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBEndpointAbort) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBEndpointAbort) ProtoMessage() {}

func (x *USBEndpointAbort) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[72]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBEndpointAbort.ProtoReflect.Descriptor instead.
func (*USBEndpointAbort) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{72}
}

func (x *USBEndpointAbort) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBEndpointAbort) GetEndpoint() uint32 {
	if x != nil {
		return x.Endpoint
	}
	return 0
}

type USBError struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	DeviceId      string                 `protobuf:"bytes,1,opt,name=deviceId,proto3" json:"deviceId,omitempty"`
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBError) Reset() {
	*x = USBError{}
	mi := &file_daemon_started_service_proto_msgTypes[73]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBError) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBError) ProtoMessage() {}

func (x *USBError) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[73]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBError.ProtoReflect.Descriptor instead.
func (*USBError) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{73}
}

func (x *USBError) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *USBError) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type USBIPServerStatusUpdate struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Servers       []*USBIPServerStatus   `protobuf:"bytes,1,rep,name=servers,proto3" json:"servers,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBIPServerStatusUpdate) Reset() {
	*x = USBIPServerStatusUpdate{}
	mi := &file_daemon_started_service_proto_msgTypes[74]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBIPServerStatusUpdate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBIPServerStatusUpdate) ProtoMessage() {}

func (x *USBIPServerStatusUpdate) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[74]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBIPServerStatusUpdate.ProtoReflect.Descriptor instead.
func (*USBIPServerStatusUpdate) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{74}
}

func (x *USBIPServerStatusUpdate) GetServers() []*USBIPServerStatus {
	if x != nil {
		return x.Servers
	}
	return nil
}

type USBIPServerStatus struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ServerTag     string                 `protobuf:"bytes,1,opt,name=serverTag,proto3" json:"serverTag,omitempty"`
	Devices       []*USBSharedDevice     `protobuf:"bytes,2,rep,name=devices,proto3" json:"devices,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBIPServerStatus) Reset() {
	*x = USBIPServerStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[75]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBIPServerStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBIPServerStatus) ProtoMessage() {}

func (x *USBIPServerStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[75]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBIPServerStatus.ProtoReflect.Descriptor instead.
func (*USBIPServerStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{75}
}

func (x *USBIPServerStatus) GetServerTag() string {
	if x != nil {
		return x.ServerTag
	}
	return ""
}

func (x *USBIPServerStatus) GetDevices() []*USBSharedDevice {
	if x != nil {
		return x.Devices
	}
	return nil
}

type USBSharedDevice struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Descriptor_   *USBDeviceDescriptor   `protobuf:"bytes,1,opt,name=descriptor,proto3" json:"descriptor,omitempty"`
	BusId         string                 `protobuf:"bytes,2,opt,name=busId,proto3" json:"busId,omitempty"`
	StableId      string                 `protobuf:"bytes,3,opt,name=stableId,proto3" json:"stableId,omitempty"`
	Backend       USBBackend             `protobuf:"varint,4,opt,name=backend,proto3,enum=daemon.USBBackend" json:"backend,omitempty"`
	State         USBDeviceState         `protobuf:"varint,5,opt,name=state,proto3,enum=daemon.USBDeviceState" json:"state,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *USBSharedDevice) Reset() {
	*x = USBSharedDevice{}
	mi := &file_daemon_started_service_proto_msgTypes[76]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *USBSharedDevice) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*USBSharedDevice) ProtoMessage() {}

func (x *USBSharedDevice) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[76]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use USBSharedDevice.ProtoReflect.Descriptor instead.
func (*USBSharedDevice) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{76}
}

func (x *USBSharedDevice) GetDescriptor_() *USBDeviceDescriptor {
	if x != nil {
		return x.Descriptor_
	}
	return nil
}

func (x *USBSharedDevice) GetBusId() string {
	if x != nil {
		return x.BusId
	}
	return ""
}

func (x *USBSharedDevice) GetStableId() string {
	if x != nil {
		return x.StableId
	}
	return ""
}

func (x *USBSharedDevice) GetBackend() USBBackend {
	if x != nil {
		return x.Backend
	}
	return USBBackend_USB_BACKEND_UNSPECIFIED
}

func (x *USBSharedDevice) GetState() USBDeviceState {
	if x != nil {
		return x.State
	}
	return USBDeviceState_USB_DEVICE_STATE_IDLE
}

type OpenConnectStatusUpdate struct {
	state         protoimpl.MessageState       `protogen:"open.v1"`
	Endpoints     []*OpenConnectEndpointStatus `protobuf:"bytes,1,rep,name=endpoints,proto3" json:"endpoints,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectStatusUpdate) Reset() {
	*x = OpenConnectStatusUpdate{}
	mi := &file_daemon_started_service_proto_msgTypes[77]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectStatusUpdate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectStatusUpdate) ProtoMessage() {}

func (x *OpenConnectStatusUpdate) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[77]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectStatusUpdate.ProtoReflect.Descriptor instead.
func (*OpenConnectStatusUpdate) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{77}
}

func (x *OpenConnectStatusUpdate) GetEndpoints() []*OpenConnectEndpointStatus {
	if x != nil {
		return x.Endpoints
	}
	return nil
}

type OpenConnectEndpointStatus struct {
	state         protoimpl.MessageState    `protogen:"open.v1"`
	EndpointTag   string                    `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	State         string                    `protobuf:"bytes,2,opt,name=state,proto3" json:"state,omitempty"`
	StateText     string                    `protobuf:"bytes,3,opt,name=stateText,proto3" json:"stateText,omitempty"`
	AuthChallenge *OpenConnectAuthChallenge `protobuf:"bytes,4,opt,name=authChallenge,proto3" json:"authChallenge,omitempty"`
	Error         string                    `protobuf:"bytes,5,opt,name=error,proto3" json:"error,omitempty"`
	TunnelInfo    *OpenConnectTunnelInfo    `protobuf:"bytes,6,opt,name=tunnelInfo,proto3" json:"tunnelInfo,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectEndpointStatus) Reset() {
	*x = OpenConnectEndpointStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[78]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectEndpointStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectEndpointStatus) ProtoMessage() {}

func (x *OpenConnectEndpointStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[78]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectEndpointStatus.ProtoReflect.Descriptor instead.
func (*OpenConnectEndpointStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{78}
}

func (x *OpenConnectEndpointStatus) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenConnectEndpointStatus) GetState() string {
	if x != nil {
		return x.State
	}
	return ""
}

func (x *OpenConnectEndpointStatus) GetStateText() string {
	if x != nil {
		return x.StateText
	}
	return ""
}

func (x *OpenConnectEndpointStatus) GetAuthChallenge() *OpenConnectAuthChallenge {
	if x != nil {
		return x.AuthChallenge
	}
	return nil
}

func (x *OpenConnectEndpointStatus) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *OpenConnectEndpointStatus) GetTunnelInfo() *OpenConnectTunnelInfo {
	if x != nil {
		return x.TunnelInfo
	}
	return nil
}

type OpenConnectTunnelInfo struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	Server         string                 `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
	Flavor         string                 `protobuf:"bytes,2,opt,name=flavor,proto3" json:"flavor,omitempty"`
	Transport      string                 `protobuf:"bytes,3,opt,name=transport,proto3" json:"transport,omitempty"`
	Ipv4           []string               `protobuf:"bytes,4,rep,name=ipv4,proto3" json:"ipv4,omitempty"`
	Ipv6           []string               `protobuf:"bytes,5,rep,name=ipv6,proto3" json:"ipv6,omitempty"`
	Dns            []string               `protobuf:"bytes,6,rep,name=dns,proto3" json:"dns,omitempty"`
	Mtu            uint32                 `protobuf:"varint,7,opt,name=mtu,proto3" json:"mtu,omitempty"`
	ConnectedSince int64                  `protobuf:"varint,8,opt,name=connectedSince,proto3" json:"connectedSince,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *OpenConnectTunnelInfo) Reset() {
	*x = OpenConnectTunnelInfo{}
	mi := &file_daemon_started_service_proto_msgTypes[79]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectTunnelInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectTunnelInfo) ProtoMessage() {}

func (x *OpenConnectTunnelInfo) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[79]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectTunnelInfo.ProtoReflect.Descriptor instead.
func (*OpenConnectTunnelInfo) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{79}
}

func (x *OpenConnectTunnelInfo) GetServer() string {
	if x != nil {
		return x.Server
	}
	return ""
}

func (x *OpenConnectTunnelInfo) GetFlavor() string {
	if x != nil {
		return x.Flavor
	}
	return ""
}

func (x *OpenConnectTunnelInfo) GetTransport() string {
	if x != nil {
		return x.Transport
	}
	return ""
}

func (x *OpenConnectTunnelInfo) GetIpv4() []string {
	if x != nil {
		return x.Ipv4
	}
	return nil
}

func (x *OpenConnectTunnelInfo) GetIpv6() []string {
	if x != nil {
		return x.Ipv6
	}
	return nil
}

func (x *OpenConnectTunnelInfo) GetDns() []string {
	if x != nil {
		return x.Dns
	}
	return nil
}

func (x *OpenConnectTunnelInfo) GetMtu() uint32 {
	if x != nil {
		return x.Mtu
	}
	return 0
}

func (x *OpenConnectTunnelInfo) GetConnectedSince() int64 {
	if x != nil {
		return x.ConnectedSince
	}
	return 0
}

type OpenConnectAuthChallenge struct {
	state   protoimpl.MessageState `protogen:"open.v1"`
	Id      string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Banner  string                 `protobuf:"bytes,2,opt,name=banner,proto3" json:"banner,omitempty"`
	Message string                 `protobuf:"bytes,3,opt,name=message,proto3" json:"message,omitempty"`
	Error   string                 `protobuf:"bytes,4,opt,name=error,proto3" json:"error,omitempty"`
	// Types that are valid to be assigned to Challenge:
	//
	//	*OpenConnectAuthChallenge_Form
	//	*OpenConnectAuthChallenge_Browser
	Challenge     isOpenConnectAuthChallenge_Challenge `protobuf_oneof:"challenge"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthChallenge) Reset() {
	*x = OpenConnectAuthChallenge{}
	mi := &file_daemon_started_service_proto_msgTypes[80]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthChallenge) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthChallenge) ProtoMessage() {}

func (x *OpenConnectAuthChallenge) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[80]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthChallenge.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthChallenge) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{80}
}

func (x *OpenConnectAuthChallenge) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *OpenConnectAuthChallenge) GetBanner() string {
	if x != nil {
		return x.Banner
	}
	return ""
}

func (x *OpenConnectAuthChallenge) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *OpenConnectAuthChallenge) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *OpenConnectAuthChallenge) GetChallenge() isOpenConnectAuthChallenge_Challenge {
	if x != nil {
		return x.Challenge
	}
	return nil
}

func (x *OpenConnectAuthChallenge) GetForm() *OpenConnectAuthForm {
	if x != nil {
		if x, ok := x.Challenge.(*OpenConnectAuthChallenge_Form); ok {
			return x.Form
		}
	}
	return nil
}

func (x *OpenConnectAuthChallenge) GetBrowser() *OpenConnectBrowserRequest {
	if x != nil {
		if x, ok := x.Challenge.(*OpenConnectAuthChallenge_Browser); ok {
			return x.Browser
		}
	}
	return nil
}

type isOpenConnectAuthChallenge_Challenge interface {
	isOpenConnectAuthChallenge_Challenge()
}

type OpenConnectAuthChallenge_Form struct {
	Form *OpenConnectAuthForm `protobuf:"bytes,5,opt,name=form,proto3,oneof"`
}

type OpenConnectAuthChallenge_Browser struct {
	Browser *OpenConnectBrowserRequest `protobuf:"bytes,6,opt,name=browser,proto3,oneof"`
}

func (*OpenConnectAuthChallenge_Form) isOpenConnectAuthChallenge_Challenge() {}

func (*OpenConnectAuthChallenge_Browser) isOpenConnectAuthChallenge_Challenge() {}

type OpenConnectAuthForm struct {
	state         protoimpl.MessageState      `protogen:"open.v1"`
	Fields        []*OpenConnectAuthFormField `protobuf:"bytes,1,rep,name=fields,proto3" json:"fields,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthForm) Reset() {
	*x = OpenConnectAuthForm{}
	mi := &file_daemon_started_service_proto_msgTypes[81]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthForm) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthForm) ProtoMessage() {}

func (x *OpenConnectAuthForm) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[81]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthForm.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthForm) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{81}
}

func (x *OpenConnectAuthForm) GetFields() []*OpenConnectAuthFormField {
	if x != nil {
		return x.Fields
	}
	return nil
}

type OpenConnectAuthFormField struct {
	state         protoimpl.MessageState       `protogen:"open.v1"`
	SubmissionKey string                       `protobuf:"bytes,1,opt,name=submissionKey,proto3" json:"submissionKey,omitempty"`
	Name          string                       `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Label         string                       `protobuf:"bytes,3,opt,name=label,proto3" json:"label,omitempty"`
	Kind          string                       `protobuf:"bytes,4,opt,name=kind,proto3" json:"kind,omitempty"`
	Value         string                       `protobuf:"bytes,5,opt,name=value,proto3" json:"value,omitempty"`
	Options       []*OpenConnectAuthFormChoice `protobuf:"bytes,6,rep,name=options,proto3" json:"options,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthFormField) Reset() {
	*x = OpenConnectAuthFormField{}
	mi := &file_daemon_started_service_proto_msgTypes[82]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthFormField) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthFormField) ProtoMessage() {}

func (x *OpenConnectAuthFormField) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[82]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthFormField.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthFormField) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{82}
}

func (x *OpenConnectAuthFormField) GetSubmissionKey() string {
	if x != nil {
		return x.SubmissionKey
	}
	return ""
}

func (x *OpenConnectAuthFormField) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *OpenConnectAuthFormField) GetLabel() string {
	if x != nil {
		return x.Label
	}
	return ""
}

func (x *OpenConnectAuthFormField) GetKind() string {
	if x != nil {
		return x.Kind
	}
	return ""
}

func (x *OpenConnectAuthFormField) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

func (x *OpenConnectAuthFormField) GetOptions() []*OpenConnectAuthFormChoice {
	if x != nil {
		return x.Options
	}
	return nil
}

type OpenConnectAuthFormChoice struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Value         string                 `protobuf:"bytes,1,opt,name=value,proto3" json:"value,omitempty"`
	Label         string                 `protobuf:"bytes,2,opt,name=label,proto3" json:"label,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthFormChoice) Reset() {
	*x = OpenConnectAuthFormChoice{}
	mi := &file_daemon_started_service_proto_msgTypes[83]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthFormChoice) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthFormChoice) ProtoMessage() {}

func (x *OpenConnectAuthFormChoice) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[83]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthFormChoice.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthFormChoice) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{83}
}

func (x *OpenConnectAuthFormChoice) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

func (x *OpenConnectAuthFormChoice) GetLabel() string {
	if x != nil {
		return x.Label
	}
	return ""
}

type OpenConnectBrowserRequest struct {
	state               protoimpl.MessageState `protogen:"open.v1"`
	Url                 string                 `protobuf:"bytes,1,opt,name=url,proto3" json:"url,omitempty"`
	FinalURL            string                 `protobuf:"bytes,2,opt,name=finalURL,proto3" json:"finalURL,omitempty"`
	CookieNames         []string               `protobuf:"bytes,3,rep,name=cookieNames,proto3" json:"cookieNames,omitempty"`
	HeaderNames         []string               `protobuf:"bytes,4,rep,name=headerNames,proto3" json:"headerNames,omitempty"`
	CallbackURLPrefixes []string               `protobuf:"bytes,5,rep,name=callbackURLPrefixes,proto3" json:"callbackURLPrefixes,omitempty"`
	EarlyCookieNames    []string               `protobuf:"bytes,6,rep,name=earlyCookieNames,proto3" json:"earlyCookieNames,omitempty"`
	CacheID             string                 `protobuf:"bytes,7,opt,name=cacheID,proto3" json:"cacheID,omitempty"`
	unknownFields       protoimpl.UnknownFields
	sizeCache           protoimpl.SizeCache
}

func (x *OpenConnectBrowserRequest) Reset() {
	*x = OpenConnectBrowserRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[84]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectBrowserRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectBrowserRequest) ProtoMessage() {}

func (x *OpenConnectBrowserRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[84]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectBrowserRequest.ProtoReflect.Descriptor instead.
func (*OpenConnectBrowserRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{84}
}

func (x *OpenConnectBrowserRequest) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

func (x *OpenConnectBrowserRequest) GetFinalURL() string {
	if x != nil {
		return x.FinalURL
	}
	return ""
}

func (x *OpenConnectBrowserRequest) GetCookieNames() []string {
	if x != nil {
		return x.CookieNames
	}
	return nil
}

func (x *OpenConnectBrowserRequest) GetHeaderNames() []string {
	if x != nil {
		return x.HeaderNames
	}
	return nil
}

func (x *OpenConnectBrowserRequest) GetCallbackURLPrefixes() []string {
	if x != nil {
		return x.CallbackURLPrefixes
	}
	return nil
}

func (x *OpenConnectBrowserRequest) GetEarlyCookieNames() []string {
	if x != nil {
		return x.EarlyCookieNames
	}
	return nil
}

func (x *OpenConnectBrowserRequest) GetCacheID() string {
	if x != nil {
		return x.CacheID
	}
	return ""
}

type OpenConnectBrowserCookie struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Value         string                 `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectBrowserCookie) Reset() {
	*x = OpenConnectBrowserCookie{}
	mi := &file_daemon_started_service_proto_msgTypes[85]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectBrowserCookie) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectBrowserCookie) ProtoMessage() {}

func (x *OpenConnectBrowserCookie) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[85]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectBrowserCookie.ProtoReflect.Descriptor instead.
func (*OpenConnectBrowserCookie) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{85}
}

func (x *OpenConnectBrowserCookie) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *OpenConnectBrowserCookie) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type OpenConnectBrowserHeader struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Values        []string               `protobuf:"bytes,2,rep,name=values,proto3" json:"values,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectBrowserHeader) Reset() {
	*x = OpenConnectBrowserHeader{}
	mi := &file_daemon_started_service_proto_msgTypes[86]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectBrowserHeader) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectBrowserHeader) ProtoMessage() {}

func (x *OpenConnectBrowserHeader) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[86]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectBrowserHeader.ProtoReflect.Descriptor instead.
func (*OpenConnectBrowserHeader) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{86}
}

func (x *OpenConnectBrowserHeader) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *OpenConnectBrowserHeader) GetValues() []string {
	if x != nil {
		return x.Values
	}
	return nil
}

type OpenConnectAuthFormResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Values        map[string]string      `protobuf:"bytes,1,rep,name=values,proto3" json:"values,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthFormResponse) Reset() {
	*x = OpenConnectAuthFormResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[87]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthFormResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthFormResponse) ProtoMessage() {}

func (x *OpenConnectAuthFormResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[87]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthFormResponse.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthFormResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{87}
}

func (x *OpenConnectAuthFormResponse) GetValues() map[string]string {
	if x != nil {
		return x.Values
	}
	return nil
}

type OpenConnectBrowserResult struct {
	state         protoimpl.MessageState      `protogen:"open.v1"`
	FinalURL      string                      `protobuf:"bytes,1,opt,name=finalURL,proto3" json:"finalURL,omitempty"`
	Cookies       []*OpenConnectBrowserCookie `protobuf:"bytes,2,rep,name=cookies,proto3" json:"cookies,omitempty"`
	Headers       []*OpenConnectBrowserHeader `protobuf:"bytes,3,rep,name=headers,proto3" json:"headers,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectBrowserResult) Reset() {
	*x = OpenConnectBrowserResult{}
	mi := &file_daemon_started_service_proto_msgTypes[88]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectBrowserResult) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectBrowserResult) ProtoMessage() {}

func (x *OpenConnectBrowserResult) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[88]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectBrowserResult.ProtoReflect.Descriptor instead.
func (*OpenConnectBrowserResult) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{88}
}

func (x *OpenConnectBrowserResult) GetFinalURL() string {
	if x != nil {
		return x.FinalURL
	}
	return ""
}

func (x *OpenConnectBrowserResult) GetCookies() []*OpenConnectBrowserCookie {
	if x != nil {
		return x.Cookies
	}
	return nil
}

func (x *OpenConnectBrowserResult) GetHeaders() []*OpenConnectBrowserHeader {
	if x != nil {
		return x.Headers
	}
	return nil
}

type OpenConnectAuthResponseSubmission struct {
	state       protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	ChallengeID string                 `protobuf:"bytes,2,opt,name=challengeID,proto3" json:"challengeID,omitempty"`
	// Types that are valid to be assigned to Response:
	//
	//	*OpenConnectAuthResponseSubmission_Form
	//	*OpenConnectAuthResponseSubmission_Browser
	Response      isOpenConnectAuthResponseSubmission_Response `protobuf_oneof:"response"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthResponseSubmission) Reset() {
	*x = OpenConnectAuthResponseSubmission{}
	mi := &file_daemon_started_service_proto_msgTypes[89]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthResponseSubmission) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthResponseSubmission) ProtoMessage() {}

func (x *OpenConnectAuthResponseSubmission) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[89]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthResponseSubmission.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthResponseSubmission) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{89}
}

func (x *OpenConnectAuthResponseSubmission) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenConnectAuthResponseSubmission) GetChallengeID() string {
	if x != nil {
		return x.ChallengeID
	}
	return ""
}

func (x *OpenConnectAuthResponseSubmission) GetResponse() isOpenConnectAuthResponseSubmission_Response {
	if x != nil {
		return x.Response
	}
	return nil
}

func (x *OpenConnectAuthResponseSubmission) GetForm() *OpenConnectAuthFormResponse {
	if x != nil {
		if x, ok := x.Response.(*OpenConnectAuthResponseSubmission_Form); ok {
			return x.Form
		}
	}
	return nil
}

func (x *OpenConnectAuthResponseSubmission) GetBrowser() *OpenConnectBrowserResult {
	if x != nil {
		if x, ok := x.Response.(*OpenConnectAuthResponseSubmission_Browser); ok {
			return x.Browser
		}
	}
	return nil
}

type isOpenConnectAuthResponseSubmission_Response interface {
	isOpenConnectAuthResponseSubmission_Response()
}

type OpenConnectAuthResponseSubmission_Form struct {
	Form *OpenConnectAuthFormResponse `protobuf:"bytes,3,opt,name=form,proto3,oneof"`
}

type OpenConnectAuthResponseSubmission_Browser struct {
	Browser *OpenConnectBrowserResult `protobuf:"bytes,4,opt,name=browser,proto3,oneof"`
}

func (*OpenConnectAuthResponseSubmission_Form) isOpenConnectAuthResponseSubmission_Response() {}

func (*OpenConnectAuthResponseSubmission_Browser) isOpenConnectAuthResponseSubmission_Response() {}

type OpenConnectAuthChallengeCancel struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	ChallengeID   string                 `protobuf:"bytes,2,opt,name=challengeID,proto3" json:"challengeID,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenConnectAuthChallengeCancel) Reset() {
	*x = OpenConnectAuthChallengeCancel{}
	mi := &file_daemon_started_service_proto_msgTypes[90]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenConnectAuthChallengeCancel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenConnectAuthChallengeCancel) ProtoMessage() {}

func (x *OpenConnectAuthChallengeCancel) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[90]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenConnectAuthChallengeCancel.ProtoReflect.Descriptor instead.
func (*OpenConnectAuthChallengeCancel) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{90}
}

func (x *OpenConnectAuthChallengeCancel) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenConnectAuthChallengeCancel) GetChallengeID() string {
	if x != nil {
		return x.ChallengeID
	}
	return ""
}

type OpenVPNStatusUpdate struct {
	state         protoimpl.MessageState   `protogen:"open.v1"`
	Endpoints     []*OpenVPNEndpointStatus `protobuf:"bytes,1,rep,name=endpoints,proto3" json:"endpoints,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenVPNStatusUpdate) Reset() {
	*x = OpenVPNStatusUpdate{}
	mi := &file_daemon_started_service_proto_msgTypes[91]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNStatusUpdate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNStatusUpdate) ProtoMessage() {}

func (x *OpenVPNStatusUpdate) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[91]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNStatusUpdate.ProtoReflect.Descriptor instead.
func (*OpenVPNStatusUpdate) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{91}
}

func (x *OpenVPNStatusUpdate) GetEndpoints() []*OpenVPNEndpointStatus {
	if x != nil {
		return x.Endpoints
	}
	return nil
}

type OpenVPNEndpointStatus struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	State         string                 `protobuf:"bytes,2,opt,name=state,proto3" json:"state,omitempty"`
	StateText     string                 `protobuf:"bytes,3,opt,name=stateText,proto3" json:"stateText,omitempty"`
	Challenge     *OpenVPNChallenge      `protobuf:"bytes,4,opt,name=challenge,proto3" json:"challenge,omitempty"`
	Error         string                 `protobuf:"bytes,5,opt,name=error,proto3" json:"error,omitempty"`
	TunnelInfo    *OpenVPNTunnelInfo     `protobuf:"bytes,6,opt,name=tunnelInfo,proto3" json:"tunnelInfo,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenVPNEndpointStatus) Reset() {
	*x = OpenVPNEndpointStatus{}
	mi := &file_daemon_started_service_proto_msgTypes[92]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNEndpointStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNEndpointStatus) ProtoMessage() {}

func (x *OpenVPNEndpointStatus) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[92]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNEndpointStatus.ProtoReflect.Descriptor instead.
func (*OpenVPNEndpointStatus) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{92}
}

func (x *OpenVPNEndpointStatus) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenVPNEndpointStatus) GetState() string {
	if x != nil {
		return x.State
	}
	return ""
}

func (x *OpenVPNEndpointStatus) GetStateText() string {
	if x != nil {
		return x.StateText
	}
	return ""
}

func (x *OpenVPNEndpointStatus) GetChallenge() *OpenVPNChallenge {
	if x != nil {
		return x.Challenge
	}
	return nil
}

func (x *OpenVPNEndpointStatus) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *OpenVPNEndpointStatus) GetTunnelInfo() *OpenVPNTunnelInfo {
	if x != nil {
		return x.TunnelInfo
	}
	return nil
}

type OpenVPNTunnelInfo struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	Server         string                 `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
	Network        string                 `protobuf:"bytes,3,opt,name=network,proto3" json:"network,omitempty"`
	Ipv4           []string               `protobuf:"bytes,4,rep,name=ipv4,proto3" json:"ipv4,omitempty"`
	Ipv6           []string               `protobuf:"bytes,5,rep,name=ipv6,proto3" json:"ipv6,omitempty"`
	Dns            []string               `protobuf:"bytes,6,rep,name=dns,proto3" json:"dns,omitempty"`
	Mtu            uint32                 `protobuf:"varint,7,opt,name=mtu,proto3" json:"mtu,omitempty"`
	ConnectedSince int64                  `protobuf:"varint,8,opt,name=connectedSince,proto3" json:"connectedSince,omitempty"`
	Cipher         string                 `protobuf:"bytes,9,opt,name=cipher,proto3" json:"cipher,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *OpenVPNTunnelInfo) Reset() {
	*x = OpenVPNTunnelInfo{}
	mi := &file_daemon_started_service_proto_msgTypes[93]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNTunnelInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNTunnelInfo) ProtoMessage() {}

func (x *OpenVPNTunnelInfo) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[93]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNTunnelInfo.ProtoReflect.Descriptor instead.
func (*OpenVPNTunnelInfo) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{93}
}

func (x *OpenVPNTunnelInfo) GetServer() string {
	if x != nil {
		return x.Server
	}
	return ""
}

func (x *OpenVPNTunnelInfo) GetNetwork() string {
	if x != nil {
		return x.Network
	}
	return ""
}

func (x *OpenVPNTunnelInfo) GetIpv4() []string {
	if x != nil {
		return x.Ipv4
	}
	return nil
}

func (x *OpenVPNTunnelInfo) GetIpv6() []string {
	if x != nil {
		return x.Ipv6
	}
	return nil
}

func (x *OpenVPNTunnelInfo) GetDns() []string {
	if x != nil {
		return x.Dns
	}
	return nil
}

func (x *OpenVPNTunnelInfo) GetMtu() uint32 {
	if x != nil {
		return x.Mtu
	}
	return 0
}

func (x *OpenVPNTunnelInfo) GetConnectedSince() int64 {
	if x != nil {
		return x.ConnectedSince
	}
	return 0
}

func (x *OpenVPNTunnelInfo) GetCipher() string {
	if x != nil {
		return x.Cipher
	}
	return ""
}

type OpenVPNChallenge struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Kind          string                 `protobuf:"bytes,2,opt,name=kind,proto3" json:"kind,omitempty"`
	Username      string                 `protobuf:"bytes,3,opt,name=username,proto3" json:"username,omitempty"`
	Message       string                 `protobuf:"bytes,4,opt,name=message,proto3" json:"message,omitempty"`
	Url           string                 `protobuf:"bytes,5,opt,name=url,proto3" json:"url,omitempty"`
	SecretMessage string                 `protobuf:"bytes,6,opt,name=secretMessage,proto3" json:"secretMessage,omitempty"`
	Echo          bool                   `protobuf:"varint,7,opt,name=echo,proto3" json:"echo,omitempty"`
	PreviousError string                 `protobuf:"bytes,8,opt,name=previousError,proto3" json:"previousError,omitempty"`
	Deadline      int64                  `protobuf:"varint,9,opt,name=deadline,proto3" json:"deadline,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenVPNChallenge) Reset() {
	*x = OpenVPNChallenge{}
	mi := &file_daemon_started_service_proto_msgTypes[94]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNChallenge) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNChallenge) ProtoMessage() {}

func (x *OpenVPNChallenge) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[94]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNChallenge.ProtoReflect.Descriptor instead.
func (*OpenVPNChallenge) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{94}
}

func (x *OpenVPNChallenge) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *OpenVPNChallenge) GetKind() string {
	if x != nil {
		return x.Kind
	}
	return ""
}

func (x *OpenVPNChallenge) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *OpenVPNChallenge) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *OpenVPNChallenge) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

func (x *OpenVPNChallenge) GetSecretMessage() string {
	if x != nil {
		return x.SecretMessage
	}
	return ""
}

func (x *OpenVPNChallenge) GetEcho() bool {
	if x != nil {
		return x.Echo
	}
	return false
}

func (x *OpenVPNChallenge) GetPreviousError() string {
	if x != nil {
		return x.PreviousError
	}
	return ""
}

func (x *OpenVPNChallenge) GetDeadline() int64 {
	if x != nil {
		return x.Deadline
	}
	return 0
}

type OpenVPNChallengeSubmission struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	ChallengeID   string                 `protobuf:"bytes,2,opt,name=challengeID,proto3" json:"challengeID,omitempty"`
	Username      string                 `protobuf:"bytes,3,opt,name=username,proto3" json:"username,omitempty"`
	Password      string                 `protobuf:"bytes,4,opt,name=password,proto3" json:"password,omitempty"`
	Secret        string                 `protobuf:"bytes,5,opt,name=secret,proto3" json:"secret,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenVPNChallengeSubmission) Reset() {
	*x = OpenVPNChallengeSubmission{}
	mi := &file_daemon_started_service_proto_msgTypes[95]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNChallengeSubmission) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNChallengeSubmission) ProtoMessage() {}

func (x *OpenVPNChallengeSubmission) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[95]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNChallengeSubmission.ProtoReflect.Descriptor instead.
func (*OpenVPNChallengeSubmission) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{95}
}

func (x *OpenVPNChallengeSubmission) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenVPNChallengeSubmission) GetChallengeID() string {
	if x != nil {
		return x.ChallengeID
	}
	return ""
}

func (x *OpenVPNChallengeSubmission) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *OpenVPNChallengeSubmission) GetPassword() string {
	if x != nil {
		return x.Password
	}
	return ""
}

func (x *OpenVPNChallengeSubmission) GetSecret() string {
	if x != nil {
		return x.Secret
	}
	return ""
}

type OpenVPNChallengeCancel struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	EndpointTag   string                 `protobuf:"bytes,1,opt,name=endpointTag,proto3" json:"endpointTag,omitempty"`
	ChallengeID   string                 `protobuf:"bytes,2,opt,name=challengeID,proto3" json:"challengeID,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenVPNChallengeCancel) Reset() {
	*x = OpenVPNChallengeCancel{}
	mi := &file_daemon_started_service_proto_msgTypes[96]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenVPNChallengeCancel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenVPNChallengeCancel) ProtoMessage() {}

func (x *OpenVPNChallengeCancel) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[96]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenVPNChallengeCancel.ProtoReflect.Descriptor instead.
func (*OpenVPNChallengeCancel) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{96}
}

func (x *OpenVPNChallengeCancel) GetEndpointTag() string {
	if x != nil {
		return x.EndpointTag
	}
	return ""
}

func (x *OpenVPNChallengeCancel) GetChallengeID() string {
	if x != nil {
		return x.ChallengeID
	}
	return ""
}

type NotificationEvent struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Event:
	//
	//	*NotificationEvent_Send
	//	*NotificationEvent_Cancel
	Event         isNotificationEvent_Event `protobuf_oneof:"event"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *NotificationEvent) Reset() {
	*x = NotificationEvent{}
	mi := &file_daemon_started_service_proto_msgTypes[97]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *NotificationEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NotificationEvent) ProtoMessage() {}

func (x *NotificationEvent) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[97]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NotificationEvent.ProtoReflect.Descriptor instead.
func (*NotificationEvent) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{97}
}

func (x *NotificationEvent) GetEvent() isNotificationEvent_Event {
	if x != nil {
		return x.Event
	}
	return nil
}

func (x *NotificationEvent) GetSend() *Notification {
	if x != nil {
		if x, ok := x.Event.(*NotificationEvent_Send); ok {
			return x.Send
		}
	}
	return nil
}

func (x *NotificationEvent) GetCancel() *NotificationCancel {
	if x != nil {
		if x, ok := x.Event.(*NotificationEvent_Cancel); ok {
			return x.Cancel
		}
	}
	return nil
}

type isNotificationEvent_Event interface {
	isNotificationEvent_Event()
}

type NotificationEvent_Send struct {
	Send *Notification `protobuf:"bytes,1,opt,name=send,proto3,oneof"`
}

type NotificationEvent_Cancel struct {
	Cancel *NotificationCancel `protobuf:"bytes,2,opt,name=cancel,proto3,oneof"`
}

func (*NotificationEvent_Send) isNotificationEvent_Event() {}

func (*NotificationEvent_Cancel) isNotificationEvent_Event() {}

type Notification struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Identifier    string                 `protobuf:"bytes,1,opt,name=identifier,proto3" json:"identifier,omitempty"`
	TypeName      string                 `protobuf:"bytes,2,opt,name=typeName,proto3" json:"typeName,omitempty"`
	TypeID        int32                  `protobuf:"varint,3,opt,name=typeID,proto3" json:"typeID,omitempty"`
	Title         string                 `protobuf:"bytes,4,opt,name=title,proto3" json:"title,omitempty"`
	Subtitle      string                 `protobuf:"bytes,5,opt,name=subtitle,proto3" json:"subtitle,omitempty"`
	Body          string                 `protobuf:"bytes,6,opt,name=body,proto3" json:"body,omitempty"`
	OpenURL       string                 `protobuf:"bytes,7,opt,name=openURL,proto3" json:"openURL,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Notification) Reset() {
	*x = Notification{}
	mi := &file_daemon_started_service_proto_msgTypes[98]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Notification) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Notification) ProtoMessage() {}

func (x *Notification) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[98]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Notification.ProtoReflect.Descriptor instead.
func (*Notification) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{98}
}

func (x *Notification) GetIdentifier() string {
	if x != nil {
		return x.Identifier
	}
	return ""
}

func (x *Notification) GetTypeName() string {
	if x != nil {
		return x.TypeName
	}
	return ""
}

func (x *Notification) GetTypeID() int32 {
	if x != nil {
		return x.TypeID
	}
	return 0
}

func (x *Notification) GetTitle() string {
	if x != nil {
		return x.Title
	}
	return ""
}

func (x *Notification) GetSubtitle() string {
	if x != nil {
		return x.Subtitle
	}
	return ""
}

func (x *Notification) GetBody() string {
	if x != nil {
		return x.Body
	}
	return ""
}

func (x *Notification) GetOpenURL() string {
	if x != nil {
		return x.OpenURL
	}
	return ""
}

type NotificationCancel struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Identifier    string                 `protobuf:"bytes,1,opt,name=identifier,proto3" json:"identifier,omitempty"`
	TypeID        int32                  `protobuf:"varint,2,opt,name=typeID,proto3" json:"typeID,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *NotificationCancel) Reset() {
	*x = NotificationCancel{}
	mi := &file_daemon_started_service_proto_msgTypes[99]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *NotificationCancel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NotificationCancel) ProtoMessage() {}

func (x *NotificationCancel) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[99]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NotificationCancel.ProtoReflect.Descriptor instead.
func (*NotificationCancel) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{99}
}

func (x *NotificationCancel) GetIdentifier() string {
	if x != nil {
		return x.Identifier
	}
	return ""
}

func (x *NotificationCancel) GetTypeID() int32 {
	if x != nil {
		return x.TypeID
	}
	return 0
}

type URLTestOutboundRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OutboundTag   string                 `protobuf:"bytes,1,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	Link          string                 `protobuf:"bytes,2,opt,name=link,proto3" json:"link,omitempty"`
	Timeout       uint32                 `protobuf:"varint,3,opt,name=timeout,proto3" json:"timeout,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *URLTestOutboundRequest) Reset() {
	*x = URLTestOutboundRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[100]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *URLTestOutboundRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*URLTestOutboundRequest) ProtoMessage() {}

func (x *URLTestOutboundRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[100]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use URLTestOutboundRequest.ProtoReflect.Descriptor instead.
func (*URLTestOutboundRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{100}
}

func (x *URLTestOutboundRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

func (x *URLTestOutboundRequest) GetLink() string {
	if x != nil {
		return x.Link
	}
	return ""
}

func (x *URLTestOutboundRequest) GetTimeout() uint32 {
	if x != nil {
		return x.Timeout
	}
	return 0
}

type URLTestOutboundResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Delay         uint32                 `protobuf:"varint,1,opt,name=delay,proto3" json:"delay,omitempty"`
	Error         string                 `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *URLTestOutboundResponse) Reset() {
	*x = URLTestOutboundResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[101]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *URLTestOutboundResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*URLTestOutboundResponse) ProtoMessage() {}

func (x *URLTestOutboundResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[101]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use URLTestOutboundResponse.ProtoReflect.Descriptor instead.
func (*URLTestOutboundResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{101}
}

func (x *URLTestOutboundResponse) GetDelay() uint32 {
	if x != nil {
		return x.Delay
	}
	return 0
}

func (x *URLTestOutboundResponse) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

// SPEC 058 — diagnostic HTTP probe through a single node. Unlike URLTestOutbound
// (which reports only a delay) this returns the response body, so the client can
// answer "which exit IP / geo / warp state does THIS node give me".
type HttpHeaderPair struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Key           string                 `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value         string                 `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *HttpHeaderPair) Reset() {
	*x = HttpHeaderPair{}
	mi := &file_daemon_started_service_proto_msgTypes[102]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *HttpHeaderPair) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HttpHeaderPair) ProtoMessage() {}

func (x *HttpHeaderPair) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[102]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HttpHeaderPair.ProtoReflect.Descriptor instead.
func (*HttpHeaderPair) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{102}
}

func (x *HttpHeaderPair) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *HttpHeaderPair) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type GetURLViaOutboundRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OutboundTag   string                 `protobuf:"bytes,1,opt,name=outboundTag,proto3" json:"outboundTag,omitempty"`
	Link          string                 `protobuf:"bytes,2,opt,name=link,proto3" json:"link,omitempty"`
	Timeout       uint32                 `protobuf:"varint,3,opt,name=timeout,proto3" json:"timeout,omitempty"`
	MaxBytes      uint32                 `protobuf:"varint,4,opt,name=maxBytes,proto3" json:"maxBytes,omitempty"`
	Headers       []*HttpHeaderPair      `protobuf:"bytes,5,rep,name=headers,proto3" json:"headers,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetURLViaOutboundRequest) Reset() {
	*x = GetURLViaOutboundRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[103]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetURLViaOutboundRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetURLViaOutboundRequest) ProtoMessage() {}

func (x *GetURLViaOutboundRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[103]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetURLViaOutboundRequest.ProtoReflect.Descriptor instead.
func (*GetURLViaOutboundRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{103}
}

func (x *GetURLViaOutboundRequest) GetOutboundTag() string {
	if x != nil {
		return x.OutboundTag
	}
	return ""
}

func (x *GetURLViaOutboundRequest) GetLink() string {
	if x != nil {
		return x.Link
	}
	return ""
}

func (x *GetURLViaOutboundRequest) GetTimeout() uint32 {
	if x != nil {
		return x.Timeout
	}
	return 0
}

func (x *GetURLViaOutboundRequest) GetMaxBytes() uint32 {
	if x != nil {
		return x.MaxBytes
	}
	return 0
}

func (x *GetURLViaOutboundRequest) GetHeaders() []*HttpHeaderPair {
	if x != nil {
		return x.Headers
	}
	return nil
}

type GetURLViaOutboundResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	HttpStatus    uint32                 `protobuf:"varint,1,opt,name=httpStatus,proto3" json:"httpStatus,omitempty"`
	Body          []byte                 `protobuf:"bytes,2,opt,name=body,proto3" json:"body,omitempty"` // bytes, not string: an arbitrary endpoint is not guaranteed valid UTF-8
	Truncated     bool                   `protobuf:"varint,3,opt,name=truncated,proto3" json:"truncated,omitempty"`
	ContentType   string                 `protobuf:"bytes,4,opt,name=contentType,proto3" json:"contentType,omitempty"`
	RemoteAddr    string                 `protobuf:"bytes,5,opt,name=remoteAddr,proto3" json:"remoteAddr,omitempty"`
	ElapsedMs     uint32                 `protobuf:"varint,6,opt,name=elapsedMs,proto3" json:"elapsedMs,omitempty"`
	Error         string                 `protobuf:"bytes,7,opt,name=error,proto3" json:"error,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetURLViaOutboundResponse) Reset() {
	*x = GetURLViaOutboundResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[104]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetURLViaOutboundResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetURLViaOutboundResponse) ProtoMessage() {}

func (x *GetURLViaOutboundResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[104]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetURLViaOutboundResponse.ProtoReflect.Descriptor instead.
func (*GetURLViaOutboundResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{104}
}

func (x *GetURLViaOutboundResponse) GetHttpStatus() uint32 {
	if x != nil {
		return x.HttpStatus
	}
	return 0
}

func (x *GetURLViaOutboundResponse) GetBody() []byte {
	if x != nil {
		return x.Body
	}
	return nil
}

func (x *GetURLViaOutboundResponse) GetTruncated() bool {
	if x != nil {
		return x.Truncated
	}
	return false
}

func (x *GetURLViaOutboundResponse) GetContentType() string {
	if x != nil {
		return x.ContentType
	}
	return ""
}

func (x *GetURLViaOutboundResponse) GetRemoteAddr() string {
	if x != nil {
		return x.RemoteAddr
	}
	return ""
}

func (x *GetURLViaOutboundResponse) GetElapsedMs() uint32 {
	if x != nil {
		return x.ElapsedMs
	}
	return 0
}

func (x *GetURLViaOutboundResponse) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

type Rule struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Type          string                 `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`
	Payload       string                 `protobuf:"bytes,2,opt,name=payload,proto3" json:"payload,omitempty"`
	Action        string                 `protobuf:"bytes,3,opt,name=action,proto3" json:"action,omitempty"`
	IsDNS         bool                   `protobuf:"varint,4,opt,name=isDNS,proto3" json:"isDNS,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Rule) Reset() {
	*x = Rule{}
	mi := &file_daemon_started_service_proto_msgTypes[105]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Rule) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Rule) ProtoMessage() {}

func (x *Rule) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[105]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Rule.ProtoReflect.Descriptor instead.
func (*Rule) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{105}
}

func (x *Rule) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *Rule) GetPayload() string {
	if x != nil {
		return x.Payload
	}
	return ""
}

func (x *Rule) GetAction() string {
	if x != nil {
		return x.Action
	}
	return ""
}

func (x *Rule) GetIsDNS() bool {
	if x != nil {
		return x.IsDNS
	}
	return false
}

type RuleList struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Rules         []*Rule                `protobuf:"bytes,1,rep,name=rules,proto3" json:"rules,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *RuleList) Reset() {
	*x = RuleList{}
	mi := &file_daemon_started_service_proto_msgTypes[106]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RuleList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RuleList) ProtoMessage() {}

func (x *RuleList) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[106]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RuleList.ProtoReflect.Descriptor instead.
func (*RuleList) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{106}
}

func (x *RuleList) GetRules() []*Rule {
	if x != nil {
		return x.Rules
	}
	return nil
}

// SPEC 018 — one structured DNS resolution. source is the resolver verb
// (exchanged/cached/optimistic/refreshed/failed); processInfo carries app attribution
// (package/uid) the text log lacks. Reuses the existing ProcessInfo message.
// failed+error cover timeout/SERVFAIL/rejected (пункт 1); rcode is -1 when there was no
// response. answers is the full response.Answer (CNAME hops + A/AAAA) in wire order, only
// populated when the subscriber set includeAnswers (пункт 2).
type SubscribeDNSQueriesRequest struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	IncludeAnswers bool                   `protobuf:"varint,1,opt,name=includeAnswers,proto3" json:"includeAnswers,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *SubscribeDNSQueriesRequest) Reset() {
	*x = SubscribeDNSQueriesRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[107]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubscribeDNSQueriesRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubscribeDNSQueriesRequest) ProtoMessage() {}

func (x *SubscribeDNSQueriesRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[107]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubscribeDNSQueriesRequest.ProtoReflect.Descriptor instead.
func (*SubscribeDNSQueriesRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{107}
}

func (x *SubscribeDNSQueriesRequest) GetIncludeAnswers() bool {
	if x != nil {
		return x.IncludeAnswers
	}
	return false
}

type DnsQueryEvent struct {
	state       protoimpl.MessageState `protogen:"open.v1"`
	Domain      string                 `protobuf:"bytes,1,opt,name=domain,proto3" json:"domain,omitempty"`
	QueryType   uint32                 `protobuf:"varint,2,opt,name=queryType,proto3" json:"queryType,omitempty"`
	Rcode       int32                  `protobuf:"varint,3,opt,name=rcode,proto3" json:"rcode,omitempty"`
	Ttl         uint32                 `protobuf:"varint,4,opt,name=ttl,proto3" json:"ttl,omitempty"`
	Source      string                 `protobuf:"bytes,5,opt,name=source,proto3" json:"source,omitempty"`
	ProcessInfo *ProcessInfo           `protobuf:"bytes,6,opt,name=processInfo,proto3" json:"processInfo,omitempty"`
	Failed      bool                   `protobuf:"varint,7,opt,name=failed,proto3" json:"failed,omitempty"`
	Error       string                 `protobuf:"bytes,8,opt,name=error,proto3" json:"error,omitempty"`
	Answers     []*DnsAnswer           `protobuf:"bytes,9,rep,name=answers,proto3" json:"answers,omitempty"`
	// SPEC 018 — which DNS server (transport) resolved this, and the outbound channel that
	// server is bound to. A selector tag is resolved to the live node via Now() server-side.
	// outbound is empty on cached/optimistic paths (the query never left the device).
	DnsServer     string   `protobuf:"bytes,10,opt,name=dnsServer,proto3" json:"dnsServer,omitempty"`
	DnsServerType string   `protobuf:"bytes,11,opt,name=dnsServerType,proto3" json:"dnsServerType,omitempty"`
	Outbound      []string `protobuf:"bytes,12,rep,name=outbound,proto3" json:"outbound,omitempty"`
	// SPEC 035 — group probe trace. dnsGroupPath is the group nesting INSIDE-OUT
	// (empty = the query did not go through a group); attempts is the probe
	// chronology snapshotted at answer time (fan stragglers that resolved later
	// are absent by design — the full picture is GetDNSGroups); fanned marks a
	// query that involved a fan-out (rescue / election / parallel); survival
	// marks an answer obtained via the least dirty server when no member was
	// clean (degradation would otherwise be invisible in the stream).
	DnsGroupPath  []string           `protobuf:"bytes,13,rep,name=dnsGroupPath,proto3" json:"dnsGroupPath,omitempty"`
	Attempts      []*DnsGroupAttempt `protobuf:"bytes,14,rep,name=attempts,proto3" json:"attempts,omitempty"`
	Fanned        bool               `protobuf:"varint,15,opt,name=fanned,proto3" json:"fanned,omitempty"`
	Survival      bool               `protobuf:"varint,16,opt,name=survival,proto3" json:"survival,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DnsQueryEvent) Reset() {
	*x = DnsQueryEvent{}
	mi := &file_daemon_started_service_proto_msgTypes[108]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsQueryEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsQueryEvent) ProtoMessage() {}

func (x *DnsQueryEvent) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[108]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsQueryEvent.ProtoReflect.Descriptor instead.
func (*DnsQueryEvent) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{108}
}

func (x *DnsQueryEvent) GetDomain() string {
	if x != nil {
		return x.Domain
	}
	return ""
}

func (x *DnsQueryEvent) GetQueryType() uint32 {
	if x != nil {
		return x.QueryType
	}
	return 0
}

func (x *DnsQueryEvent) GetRcode() int32 {
	if x != nil {
		return x.Rcode
	}
	return 0
}

func (x *DnsQueryEvent) GetTtl() uint32 {
	if x != nil {
		return x.Ttl
	}
	return 0
}

func (x *DnsQueryEvent) GetSource() string {
	if x != nil {
		return x.Source
	}
	return ""
}

func (x *DnsQueryEvent) GetProcessInfo() *ProcessInfo {
	if x != nil {
		return x.ProcessInfo
	}
	return nil
}

func (x *DnsQueryEvent) GetFailed() bool {
	if x != nil {
		return x.Failed
	}
	return false
}

func (x *DnsQueryEvent) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *DnsQueryEvent) GetAnswers() []*DnsAnswer {
	if x != nil {
		return x.Answers
	}
	return nil
}

func (x *DnsQueryEvent) GetDnsServer() string {
	if x != nil {
		return x.DnsServer
	}
	return ""
}

func (x *DnsQueryEvent) GetDnsServerType() string {
	if x != nil {
		return x.DnsServerType
	}
	return ""
}

func (x *DnsQueryEvent) GetOutbound() []string {
	if x != nil {
		return x.Outbound
	}
	return nil
}

func (x *DnsQueryEvent) GetDnsGroupPath() []string {
	if x != nil {
		return x.DnsGroupPath
	}
	return nil
}

func (x *DnsQueryEvent) GetAttempts() []*DnsGroupAttempt {
	if x != nil {
		return x.Attempts
	}
	return nil
}

func (x *DnsQueryEvent) GetFanned() bool {
	if x != nil {
		return x.Fanned
	}
	return false
}

func (x *DnsQueryEvent) GetSurvival() bool {
	if x != nil {
		return x.Survival
	}
	return false
}

// One resolved member probe of a DNS group (SPEC 035). outcome vocabulary:
// answered | timeout | network_error | servfail ("answered" includes NXDOMAIN
// and empty answers — they are valid responses, not failures).
type DnsGroupAttempt struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Server        string                 `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
	ServerType    string                 `protobuf:"bytes,2,opt,name=serverType,proto3" json:"serverType,omitempty"`
	Outcome       string                 `protobuf:"bytes,3,opt,name=outcome,proto3" json:"outcome,omitempty"`
	RttMs         uint32                 `protobuf:"varint,4,opt,name=rttMs,proto3" json:"rttMs,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DnsGroupAttempt) Reset() {
	*x = DnsGroupAttempt{}
	mi := &file_daemon_started_service_proto_msgTypes[109]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsGroupAttempt) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsGroupAttempt) ProtoMessage() {}

func (x *DnsGroupAttempt) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[109]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsGroupAttempt.ProtoReflect.Descriptor instead.
func (*DnsGroupAttempt) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{109}
}

func (x *DnsGroupAttempt) GetServer() string {
	if x != nil {
		return x.Server
	}
	return ""
}

func (x *DnsGroupAttempt) GetServerType() string {
	if x != nil {
		return x.ServerType
	}
	return ""
}

func (x *DnsGroupAttempt) GetOutcome() string {
	if x != nil {
		return x.Outcome
	}
	return ""
}

func (x *DnsGroupAttempt) GetRttMs() uint32 {
	if x != nil {
		return x.RttMs
	}
	return 0
}

type DnsAnswer struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Type          uint32                 `protobuf:"varint,2,opt,name=type,proto3" json:"type,omitempty"`
	Rdata         string                 `protobuf:"bytes,3,opt,name=rdata,proto3" json:"rdata,omitempty"`
	Ttl           uint32                 `protobuf:"varint,4,opt,name=ttl,proto3" json:"ttl,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DnsAnswer) Reset() {
	*x = DnsAnswer{}
	mi := &file_daemon_started_service_proto_msgTypes[110]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsAnswer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsAnswer) ProtoMessage() {}

func (x *DnsAnswer) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[110]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsAnswer.ProtoReflect.Descriptor instead.
func (*DnsAnswer) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{110}
}

func (x *DnsAnswer) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *DnsAnswer) GetType() uint32 {
	if x != nil {
		return x.Type
	}
	return 0
}

func (x *DnsAnswer) GetRdata() string {
	if x != nil {
		return x.Rdata
	}
	return ""
}

func (x *DnsAnswer) GetTtl() uint32 {
	if x != nil {
		return x.Ttl
	}
	return 0
}

// SPEC 019 v2 — the round_robin rotation pool of a urltest group. One PoolSlot per fixed
// slot. delay is the node's last test result in ms; 0 means dead/not-measured (a live node
// is clamped to >= 1 server-side). A non-round_robin group (selector/least_test) returns an
// empty slots list — "this group has no pool", not an error.
type GetPoolRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	GroupTag      string                 `protobuf:"bytes,1,opt,name=groupTag,proto3" json:"groupTag,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetPoolRequest) Reset() {
	*x = GetPoolRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[111]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetPoolRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetPoolRequest) ProtoMessage() {}

func (x *GetPoolRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[111]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetPoolRequest.ProtoReflect.Descriptor instead.
func (*GetPoolRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{111}
}

func (x *GetPoolRequest) GetGroupTag() string {
	if x != nil {
		return x.GroupTag
	}
	return ""
}

type PoolSlot struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Slot          uint32                 `protobuf:"varint,1,opt,name=slot,proto3" json:"slot,omitempty"`
	Tag           string                 `protobuf:"bytes,2,opt,name=tag,proto3" json:"tag,omitempty"`
	Delay         uint32                 `protobuf:"varint,3,opt,name=delay,proto3" json:"delay,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PoolSlot) Reset() {
	*x = PoolSlot{}
	mi := &file_daemon_started_service_proto_msgTypes[112]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PoolSlot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PoolSlot) ProtoMessage() {}

func (x *PoolSlot) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[112]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PoolSlot.ProtoReflect.Descriptor instead.
func (*PoolSlot) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{112}
}

func (x *PoolSlot) GetSlot() uint32 {
	if x != nil {
		return x.Slot
	}
	return 0
}

func (x *PoolSlot) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *PoolSlot) GetDelay() uint32 {
	if x != nil {
		return x.Delay
	}
	return 0
}

type PoolList struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Slots         []*PoolSlot            `protobuf:"bytes,1,rep,name=slots,proto3" json:"slots,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PoolList) Reset() {
	*x = PoolList{}
	mi := &file_daemon_started_service_proto_msgTypes[113]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PoolList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PoolList) ProtoMessage() {}

func (x *PoolList) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[113]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PoolList.ProtoReflect.Descriptor instead.
func (*PoolList) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{113}
}

func (x *PoolList) GetSlots() []*PoolSlot {
	if x != nil {
		return x.Slots
	}
	return nil
}

// SPEC 035 v3 — point-in-time record snapshot of every DNS group in the
// config (groups are few; the UI draws them all, so there is no per-tag
// request). clean = zero live errors; lastErrorAgeMs: age of the newest live
// error, -1 = none; liveWins: live win records (fastest mode);
// current = the group's sticky target; lastRttMs: last successful probe,
// 0 = never measured. The UI derives "when does it clear" from the ages and
// the error_ttl/win_ttl it knows from the config.
type DnsGroupMember struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	Tag            string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	ServerType     string                 `protobuf:"bytes,2,opt,name=serverType,proto3" json:"serverType,omitempty"`
	Clean          bool                   `protobuf:"varint,3,opt,name=clean,proto3" json:"clean,omitempty"`
	LiveErrors     uint32                 `protobuf:"varint,4,opt,name=liveErrors,proto3" json:"liveErrors,omitempty"`
	LastErrorAgeMs int64                  `protobuf:"varint,5,opt,name=lastErrorAgeMs,proto3" json:"lastErrorAgeMs,omitempty"`
	LiveWins       uint32                 `protobuf:"varint,6,opt,name=liveWins,proto3" json:"liveWins,omitempty"`
	Current        bool                   `protobuf:"varint,7,opt,name=current,proto3" json:"current,omitempty"`
	LastRttMs      uint32                 `protobuf:"varint,8,opt,name=lastRttMs,proto3" json:"lastRttMs,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *DnsGroupMember) Reset() {
	*x = DnsGroupMember{}
	mi := &file_daemon_started_service_proto_msgTypes[114]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsGroupMember) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsGroupMember) ProtoMessage() {}

func (x *DnsGroupMember) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[114]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsGroupMember.ProtoReflect.Descriptor instead.
func (*DnsGroupMember) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{114}
}

func (x *DnsGroupMember) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *DnsGroupMember) GetServerType() string {
	if x != nil {
		return x.ServerType
	}
	return ""
}

func (x *DnsGroupMember) GetClean() bool {
	if x != nil {
		return x.Clean
	}
	return false
}

func (x *DnsGroupMember) GetLiveErrors() uint32 {
	if x != nil {
		return x.LiveErrors
	}
	return 0
}

func (x *DnsGroupMember) GetLastErrorAgeMs() int64 {
	if x != nil {
		return x.LastErrorAgeMs
	}
	return 0
}

func (x *DnsGroupMember) GetLiveWins() uint32 {
	if x != nil {
		return x.LiveWins
	}
	return 0
}

func (x *DnsGroupMember) GetCurrent() bool {
	if x != nil {
		return x.Current
	}
	return false
}

func (x *DnsGroupMember) GetLastRttMs() uint32 {
	if x != nil {
		return x.LastRttMs
	}
	return 0
}

type DnsGroupState struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Tag           string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	Mode          string                 `protobuf:"bytes,2,opt,name=mode,proto3" json:"mode,omitempty"`
	Current       string                 `protobuf:"bytes,3,opt,name=current,proto3" json:"current,omitempty"`
	Members       []*DnsGroupMember      `protobuf:"bytes,6,rep,name=members,proto3" json:"members,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DnsGroupState) Reset() {
	*x = DnsGroupState{}
	mi := &file_daemon_started_service_proto_msgTypes[115]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsGroupState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsGroupState) ProtoMessage() {}

func (x *DnsGroupState) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[115]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsGroupState.ProtoReflect.Descriptor instead.
func (*DnsGroupState) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{115}
}

func (x *DnsGroupState) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *DnsGroupState) GetMode() string {
	if x != nil {
		return x.Mode
	}
	return ""
}

func (x *DnsGroupState) GetCurrent() string {
	if x != nil {
		return x.Current
	}
	return ""
}

func (x *DnsGroupState) GetMembers() []*DnsGroupMember {
	if x != nil {
		return x.Members
	}
	return nil
}

type DnsGroupList struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Groups        []*DnsGroupState       `protobuf:"bytes,1,rep,name=groups,proto3" json:"groups,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DnsGroupList) Reset() {
	*x = DnsGroupList{}
	mi := &file_daemon_started_service_proto_msgTypes[116]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DnsGroupList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DnsGroupList) ProtoMessage() {}

func (x *DnsGroupList) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[116]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DnsGroupList.ProtoReflect.Descriptor instead.
func (*DnsGroupList) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{116}
}

func (x *DnsGroupList) GetGroups() []*DnsGroupState {
	if x != nil {
		return x.Groups
	}
	return nil
}

// SPEC 037 — the canonical serialization of the options the running box was actually
// built from: post-override (tun AutoRedirect/packages, injected OOM-killer service),
// re-marshaled from the parsed struct. NOT byte-identical to the profile text the client
// sent (field order, omitempty, [] -> null normalization) — compare semantically, not
// textually. Serialized once at service start; this is a cheap string handoff.
type RunningConfig struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Content       string                 `protobuf:"bytes,1,opt,name=content,proto3" json:"content,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *RunningConfig) Reset() {
	*x = RunningConfig{}
	mi := &file_daemon_started_service_proto_msgTypes[117]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RunningConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RunningConfig) ProtoMessage() {}

func (x *RunningConfig) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[117]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RunningConfig.ProtoReflect.Descriptor instead.
func (*RunningConfig) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{117}
}

func (x *RunningConfig) GetContent() string {
	if x != nil {
		return x.Content
	}
	return ""
}

// SPEC 073 — state of every `chain` outbound: per position the resolved node
// (`now`; `transparent` = direct at position >= 1 collapses the hop) and, for
// positions >= 1, the live link instance ("clone") of that node: state
// starting|active|idle, live connections, ages, effective MTU and why,
// what `strip` removed and whether `rewrite` applied, last dial error.
type ChainCloneState struct {
	state           protoimpl.MessageState `protogen:"open.v1"`
	State           string                 `protobuf:"bytes,1,opt,name=state,proto3" json:"state,omitempty"`
	ActiveConns     int64                  `protobuf:"varint,2,opt,name=activeConns,proto3" json:"activeConns,omitempty"`
	LastPickedAgeMs int64                  `protobuf:"varint,3,opt,name=lastPickedAgeMs,proto3" json:"lastPickedAgeMs,omitempty"`
	CreatedAgeMs    int64                  `protobuf:"varint,4,opt,name=createdAgeMs,proto3" json:"createdAgeMs,omitempty"`
	MtuConfigured   uint32                 `protobuf:"varint,5,opt,name=mtuConfigured,proto3" json:"mtuConfigured,omitempty"`
	MtuEffective    uint32                 `protobuf:"varint,6,opt,name=mtuEffective,proto3" json:"mtuEffective,omitempty"`
	MtuReason       string                 `protobuf:"bytes,7,opt,name=mtuReason,proto3" json:"mtuReason,omitempty"`
	Stripped        []string               `protobuf:"bytes,8,rep,name=stripped,proto3" json:"stripped,omitempty"`
	Rewritten       bool                   `protobuf:"varint,9,opt,name=rewritten,proto3" json:"rewritten,omitempty"`
	LastError       string                 `protobuf:"bytes,10,opt,name=lastError,proto3" json:"lastError,omitempty"`
	unknownFields   protoimpl.UnknownFields
	sizeCache       protoimpl.SizeCache
}

func (x *ChainCloneState) Reset() {
	*x = ChainCloneState{}
	mi := &file_daemon_started_service_proto_msgTypes[118]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ChainCloneState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainCloneState) ProtoMessage() {}

func (x *ChainCloneState) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[118]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainCloneState.ProtoReflect.Descriptor instead.
func (*ChainCloneState) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{118}
}

func (x *ChainCloneState) GetState() string {
	if x != nil {
		return x.State
	}
	return ""
}

func (x *ChainCloneState) GetActiveConns() int64 {
	if x != nil {
		return x.ActiveConns
	}
	return 0
}

func (x *ChainCloneState) GetLastPickedAgeMs() int64 {
	if x != nil {
		return x.LastPickedAgeMs
	}
	return 0
}

func (x *ChainCloneState) GetCreatedAgeMs() int64 {
	if x != nil {
		return x.CreatedAgeMs
	}
	return 0
}

func (x *ChainCloneState) GetMtuConfigured() uint32 {
	if x != nil {
		return x.MtuConfigured
	}
	return 0
}

func (x *ChainCloneState) GetMtuEffective() uint32 {
	if x != nil {
		return x.MtuEffective
	}
	return 0
}

func (x *ChainCloneState) GetMtuReason() string {
	if x != nil {
		return x.MtuReason
	}
	return ""
}

func (x *ChainCloneState) GetStripped() []string {
	if x != nil {
		return x.Stripped
	}
	return nil
}

func (x *ChainCloneState) GetRewritten() bool {
	if x != nil {
		return x.Rewritten
	}
	return false
}

func (x *ChainCloneState) GetLastError() string {
	if x != nil {
		return x.LastError
	}
	return ""
}

type ChainPosition struct {
	state       protoimpl.MessageState `protogen:"open.v1"`
	Tag         string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	IsGroup     bool                   `protobuf:"varint,2,opt,name=isGroup,proto3" json:"isGroup,omitempty"`
	Now         string                 `protobuf:"bytes,3,opt,name=now,proto3" json:"now,omitempty"`
	Transparent bool                   `protobuf:"varint,4,opt,name=transparent,proto3" json:"transparent,omitempty"`
	Errors      int64                  `protobuf:"varint,5,opt,name=errors,proto3" json:"errors,omitempty"`
	Clone       *ChainCloneState       `protobuf:"bytes,6,opt,name=clone,proto3" json:"clone,omitempty"`
	// SPEC 075 — runtime toggle: the position is excluded from the path (`now`
	// stays filled so diagnostics can show WHAT is disabled; clone is nil).
	Disabled      bool `protobuf:"varint,7,opt,name=disabled,proto3" json:"disabled,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ChainPosition) Reset() {
	*x = ChainPosition{}
	mi := &file_daemon_started_service_proto_msgTypes[119]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ChainPosition) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainPosition) ProtoMessage() {}

func (x *ChainPosition) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[119]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainPosition.ProtoReflect.Descriptor instead.
func (*ChainPosition) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{119}
}

func (x *ChainPosition) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *ChainPosition) GetIsGroup() bool {
	if x != nil {
		return x.IsGroup
	}
	return false
}

func (x *ChainPosition) GetNow() string {
	if x != nil {
		return x.Now
	}
	return ""
}

func (x *ChainPosition) GetTransparent() bool {
	if x != nil {
		return x.Transparent
	}
	return false
}

func (x *ChainPosition) GetErrors() int64 {
	if x != nil {
		return x.Errors
	}
	return 0
}

func (x *ChainPosition) GetClone() *ChainCloneState {
	if x != nil {
		return x.Clone
	}
	return nil
}

func (x *ChainPosition) GetDisabled() bool {
	if x != nil {
		return x.Disabled
	}
	return false
}

type ChainState struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Tag           string                 `protobuf:"bytes,1,opt,name=tag,proto3" json:"tag,omitempty"`
	Positions     []*ChainPosition       `protobuf:"bytes,2,rep,name=positions,proto3" json:"positions,omitempty"`
	Dials         int64                  `protobuf:"varint,3,opt,name=dials,proto3" json:"dials,omitempty"`
	Errors        int64                  `protobuf:"varint,4,opt,name=errors,proto3" json:"errors,omitempty"`
	ClonesCreated int64                  `protobuf:"varint,5,opt,name=clonesCreated,proto3" json:"clonesCreated,omitempty"`
	ClonesEvicted int64                  `protobuf:"varint,6,opt,name=clonesEvicted,proto3" json:"clonesEvicted,omitempty"`
	LiveClones    int64                  `protobuf:"varint,7,opt,name=liveClones,proto3" json:"liveClones,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ChainState) Reset() {
	*x = ChainState{}
	mi := &file_daemon_started_service_proto_msgTypes[120]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ChainState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainState) ProtoMessage() {}

func (x *ChainState) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[120]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainState.ProtoReflect.Descriptor instead.
func (*ChainState) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{120}
}

func (x *ChainState) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *ChainState) GetPositions() []*ChainPosition {
	if x != nil {
		return x.Positions
	}
	return nil
}

func (x *ChainState) GetDials() int64 {
	if x != nil {
		return x.Dials
	}
	return 0
}

func (x *ChainState) GetErrors() int64 {
	if x != nil {
		return x.Errors
	}
	return 0
}

func (x *ChainState) GetClonesCreated() int64 {
	if x != nil {
		return x.ClonesCreated
	}
	return 0
}

func (x *ChainState) GetClonesEvicted() int64 {
	if x != nil {
		return x.ClonesEvicted
	}
	return 0
}

func (x *ChainState) GetLiveClones() int64 {
	if x != nil {
		return x.LiveClones
	}
	return 0
}

type ChainList struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Chains        []*ChainState          `protobuf:"bytes,1,rep,name=chains,proto3" json:"chains,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ChainList) Reset() {
	*x = ChainList{}
	mi := &file_daemon_started_service_proto_msgTypes[121]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ChainList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainList) ProtoMessage() {}

func (x *ChainList) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[121]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainList.ProtoReflect.Descriptor instead.
func (*ChainList) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{121}
}

func (x *ChainList) GetChains() []*ChainState {
	if x != nil {
		return x.Chains
	}
	return nil
}

// SPEC 075 — runtime enable/disable of one chain position (packet order,
// 0 = entry). Any combination is valid: all positions disabled degenerates the
// chain into direct. The flag always applies; a failed link warm-up on enable
// is DATA (warmupError), not a status error — status errors are reserved for
// genuine call failures (unknown chain, index out of range, service stopped).
type SetChainPositionEnabledRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ChainTag      string                 `protobuf:"bytes,1,opt,name=chainTag,proto3" json:"chainTag,omitempty"`
	Position      int32                  `protobuf:"varint,2,opt,name=position,proto3" json:"position,omitempty"`
	Enabled       bool                   `protobuf:"varint,3,opt,name=enabled,proto3" json:"enabled,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetChainPositionEnabledRequest) Reset() {
	*x = SetChainPositionEnabledRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[122]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetChainPositionEnabledRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetChainPositionEnabledRequest) ProtoMessage() {}

func (x *SetChainPositionEnabledRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[122]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetChainPositionEnabledRequest.ProtoReflect.Descriptor instead.
func (*SetChainPositionEnabledRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{122}
}

func (x *SetChainPositionEnabledRequest) GetChainTag() string {
	if x != nil {
		return x.ChainTag
	}
	return ""
}

func (x *SetChainPositionEnabledRequest) GetPosition() int32 {
	if x != nil {
		return x.Position
	}
	return 0
}

func (x *SetChainPositionEnabledRequest) GetEnabled() bool {
	if x != nil {
		return x.Enabled
	}
	return false
}

type SetChainPositionEnabledResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	WarmupError   string                 `protobuf:"bytes,1,opt,name=warmupError,proto3" json:"warmupError,omitempty"` // "" = ok, or warm-up not applicable (urltest/direct/disable)
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SetChainPositionEnabledResponse) Reset() {
	*x = SetChainPositionEnabledResponse{}
	mi := &file_daemon_started_service_proto_msgTypes[123]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SetChainPositionEnabledResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SetChainPositionEnabledResponse) ProtoMessage() {}

func (x *SetChainPositionEnabledResponse) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[123]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SetChainPositionEnabledResponse.ProtoReflect.Descriptor instead.
func (*SetChainPositionEnabledResponse) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{123}
}

func (x *SetChainPositionEnabledResponse) GetWarmupError() string {
	if x != nil {
		return x.WarmupError
	}
	return ""
}

// SPEC 075 — effective post-transform options JSON ({type, tag, ...} after
// strip/rewrite/MTU/detour) of the live link at the position's currently
// resolved leaf. Snapshotted at clone creation (RunningConfig model, SPEC 037).
// NotFound when no live link exists (position 0, transparent, evicted, disabled).
type GetChainCloneConfigRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	ChainTag      string                 `protobuf:"bytes,1,opt,name=chainTag,proto3" json:"chainTag,omitempty"`
	Position      int32                  `protobuf:"varint,2,opt,name=position,proto3" json:"position,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetChainCloneConfigRequest) Reset() {
	*x = GetChainCloneConfigRequest{}
	mi := &file_daemon_started_service_proto_msgTypes[124]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetChainCloneConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetChainCloneConfigRequest) ProtoMessage() {}

func (x *GetChainCloneConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[124]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetChainCloneConfigRequest.ProtoReflect.Descriptor instead.
func (*GetChainCloneConfigRequest) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{124}
}

func (x *GetChainCloneConfigRequest) GetChainTag() string {
	if x != nil {
		return x.ChainTag
	}
	return ""
}

func (x *GetChainCloneConfigRequest) GetPosition() int32 {
	if x != nil {
		return x.Position
	}
	return 0
}

type Log_Message struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Level         LogLevel               `protobuf:"varint,1,opt,name=level,proto3,enum=daemon.LogLevel" json:"level,omitempty"`
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Log_Message) Reset() {
	*x = Log_Message{}
	mi := &file_daemon_started_service_proto_msgTypes[125]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Log_Message) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Log_Message) ProtoMessage() {}

func (x *Log_Message) ProtoReflect() protoreflect.Message {
	mi := &file_daemon_started_service_proto_msgTypes[125]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Log_Message.ProtoReflect.Descriptor instead.
func (*Log_Message) Descriptor() ([]byte, []int) {
	return file_daemon_started_service_proto_rawDescGZIP(), []int{3, 0}
}

func (x *Log_Message) GetLevel() LogLevel {
	if x != nil {
		return x.Level
	}
	return LogLevel_PANIC
}

func (x *Log_Message) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

var File_daemon_started_service_proto protoreflect.FileDescriptor

const file_daemon_started_service_proto_rawDesc = "" +
	"\n" +
	"\x1cdaemon/started_service.proto\x12\x06daemon\x1a\x1bgoogle/protobuf/empty.proto\"C\n" +
	"\aVersion\x12\x18\n" +
	"\aversion\x18\x01 \x01(\tR\aversion\x12\x1e\n" +
	"\n" +
	"apiVersion\x18\x02 \x01(\x05R\n" +
	"apiVersion\"\xad\x01\n" +
	"\rServiceStatus\x122\n" +
	"\x06status\x18\x01 \x01(\x0e2\x1a.daemon.ServiceStatus.TypeR\x06status\x12\"\n" +
	"\ferrorMessage\x18\x02 \x01(\tR\ferrorMessage\"D\n" +
	"\x04Type\x12\b\n" +
	"\x04IDLE\x10\x00\x12\f\n" +
	"\bSTARTING\x10\x01\x12\v\n" +
	"\aSTARTED\x10\x02\x12\f\n" +
	"\bSTOPPING\x10\x03\x12\t\n" +
	"\x05FATAL\x10\x04\"4\n" +
	"\x16SubscribeStatusRequest\x12\x1a\n" +
	"\binterval\x18\x01 \x01(\x03R\binterval\"\x99\x01\n" +
	"\x03Log\x12/\n" +
	"\bmessages\x18\x01 \x03(\v2\x13.daemon.Log.MessageR\bmessages\x12\x14\n" +
	"\x05reset\x18\x02 \x01(\bR\x05reset\x1aK\n" +
	"\aMessage\x12&\n" +
	"\x05level\x18\x01 \x01(\x0e2\x10.daemon.LogLevelR\x05level\x12\x18\n" +
	"\amessage\x18\x02 \x01(\tR\amessage\"9\n" +
	"\x0fDefaultLogLevel\x12&\n" +
	"\x05level\x18\x01 \x01(\x0e2\x10.daemon.LogLevelR\x05level\"\xb6\x02\n" +
	"\x06Status\x12\x16\n" +
	"\x06memory\x18\x01 \x01(\x04R\x06memory\x12\x1e\n" +
	"\n" +
	"goroutines\x18\x02 \x01(\x05R\n" +
	"goroutines\x12$\n" +
	"\rconnectionsIn\x18\x03 \x01(\x05R\rconnectionsIn\x12&\n" +
	"\x0econnectionsOut\x18\x04 \x01(\x05R\x0econnectionsOut\x12*\n" +
	"\x10trafficAvailable\x18\x05 \x01(\bR\x10trafficAvailable\x12\x16\n" +
	"\x06uplink\x18\x06 \x01(\x03R\x06uplink\x12\x1a\n" +
	"\bdownlink\x18\a \x01(\x03R\bdownlink\x12 \n" +
	"\vuplinkTotal\x18\b \x01(\x03R\vuplinkTotal\x12$\n" +
	"\rdownlinkTotal\x18\t \x01(\x03R\rdownlinkTotal\"-\n" +
	"\x06Groups\x12#\n" +
	"\x05group\x18\x01 \x03(\v2\r.daemon.GroupR\x05group\"\xc2\x01\n" +
	"\x05Group\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x12\x12\n" +
	"\x04type\x18\x02 \x01(\tR\x04type\x12\x1e\n" +
	"\n" +
	"selectable\x18\x03 \x01(\bR\n" +
	"selectable\x12\x1a\n" +
	"\bselected\x18\x04 \x01(\tR\bselected\x12\x1a\n" +
	"\bisExpand\x18\x05 \x01(\bR\bisExpand\x12'\n" +
	"\x05items\x18\x06 \x03(\v2\x11.daemon.GroupItemR\x05items\x12\x12\n" +
	"\x04mode\x18\a \x01(\tR\x04mode\"w\n" +
	"\tGroupItem\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x12\x12\n" +
	"\x04type\x18\x02 \x01(\tR\x04type\x12 \n" +
	"\vurlTestTime\x18\x03 \x01(\x03R\vurlTestTime\x12\"\n" +
	"\furlTestDelay\x18\x04 \x01(\x05R\furlTestDelay\"2\n" +
	"\x0eURLTestRequest\x12 \n" +
	"\voutboundTag\x18\x01 \x01(\tR\voutboundTag\"U\n" +
	"\x15SelectOutboundRequest\x12\x1a\n" +
	"\bgroupTag\x18\x01 \x01(\tR\bgroupTag\x12 \n" +
	"\voutboundTag\x18\x02 \x01(\tR\voutboundTag\"O\n" +
	"\x15SetGroupExpandRequest\x12\x1a\n" +
	"\bgroupTag\x18\x01 \x01(\tR\bgroupTag\x12\x1a\n" +
	"\bisExpand\x18\x02 \x01(\bR\bisExpand\"\x1f\n" +
	"\tClashMode\x12\x12\n" +
	"\x04mode\x18\x03 \x01(\tR\x04mode\"O\n" +
	"\x0fClashModeStatus\x12\x1a\n" +
	"\bmodeList\x18\x01 \x03(\tR\bmodeList\x12 \n" +
	"\vcurrentMode\x18\x02 \x01(\tR\vcurrentMode\"9\n" +
	"\x1bSubscribeConnectionsRequest\x12\x1a\n" +
	"\binterval\x18\x01 \x01(\x03R\binterval\"\xea\x01\n" +
	"\x0fConnectionEvent\x12/\n" +
	"\x04type\x18\x01 \x01(\x0e2\x1b.daemon.ConnectionEventTypeR\x04type\x12\x0e\n" +
	"\x02id\x18\x02 \x01(\tR\x02id\x122\n" +
	"\n" +
	"connection\x18\x03 \x01(\v2\x12.daemon.ConnectionR\n" +
	"connection\x12 \n" +
	"\vuplinkDelta\x18\x04 \x01(\x03R\vuplinkDelta\x12$\n" +
	"\rdownlinkDelta\x18\x05 \x01(\x03R\rdownlinkDelta\x12\x1a\n" +
	"\bclosedAt\x18\x06 \x01(\x03R\bclosedAt\"Y\n" +
	"\x10ConnectionEvents\x12/\n" +
	"\x06events\x18\x01 \x03(\v2\x17.daemon.ConnectionEventR\x06events\x12\x14\n" +
	"\x05reset\x18\x02 \x01(\bR\x05reset\"\xb5\x05\n" +
	"\n" +
	"Connection\x12\x0e\n" +
	"\x02id\x18\x01 \x01(\tR\x02id\x12\x18\n" +
	"\ainbound\x18\x02 \x01(\tR\ainbound\x12 \n" +
	"\vinboundType\x18\x03 \x01(\tR\vinboundType\x12\x1c\n" +
	"\tipVersion\x18\x04 \x01(\x05R\tipVersion\x12\x18\n" +
	"\anetwork\x18\x05 \x01(\tR\anetwork\x12\x16\n" +
	"\x06source\x18\x06 \x01(\tR\x06source\x12 \n" +
	"\vdestination\x18\a \x01(\tR\vdestination\x12\x16\n" +
	"\x06domain\x18\b \x01(\tR\x06domain\x12\x1a\n" +
	"\bprotocol\x18\t \x01(\tR\bprotocol\x12\x12\n" +
	"\x04user\x18\n" +
	" \x01(\tR\x04user\x12\"\n" +
	"\ffromOutbound\x18\v \x01(\tR\ffromOutbound\x12\x1c\n" +
	"\tcreatedAt\x18\f \x01(\x03R\tcreatedAt\x12\x1a\n" +
	"\bclosedAt\x18\r \x01(\x03R\bclosedAt\x12\x16\n" +
	"\x06uplink\x18\x0e \x01(\x03R\x06uplink\x12\x1a\n" +
	"\bdownlink\x18\x0f \x01(\x03R\bdownlink\x12 \n" +
	"\vuplinkTotal\x18\x10 \x01(\x03R\vuplinkTotal\x12$\n" +
	"\rdownlinkTotal\x18\x11 \x01(\x03R\rdownlinkTotal\x12\x12\n" +
	"\x04rule\x18\x12 \x01(\tR\x04rule\x12\x1a\n" +
	"\boutbound\x18\x13 \x01(\tR\boutbound\x12\"\n" +
	"\foutboundType\x18\x14 \x01(\tR\foutboundType\x12\x1c\n" +
	"\tchainList\x18\x15 \x03(\tR\tchainList\x125\n" +
	"\vprocessInfo\x18\x16 \x01(\v2\x13.daemon.ProcessInfoR\vprocessInfo\x12\x1e\n" +
	"\n" +
	"detourList\x18\x17 \x03(\tR\n" +
	"detourList\"\xa5\x01\n" +
	"\vProcessInfo\x12\x1c\n" +
	"\tprocessId\x18\x01 \x01(\rR\tprocessId\x12\x16\n" +
	"\x06userId\x18\x02 \x01(\x05R\x06userId\x12\x1a\n" +
	"\buserName\x18\x03 \x01(\tR\buserName\x12 \n" +
	"\vprocessPath\x18\x04 \x01(\tR\vprocessPath\x12\"\n" +
	"\fpackageNames\x18\x05 \x03(\tR\fpackageNames\"(\n" +
	"\x16CloseConnectionRequest\x12\x0e\n" +
	"\x02id\x18\x01 \x01(\tR\x02id\"K\n" +
	"\x12DeprecatedWarnings\x125\n" +
	"\bwarnings\x18\x01 \x03(\v2\x19.daemon.DeprecatedWarningR\bwarnings\"\xed\x01\n" +
	"\x11DeprecatedWarning\x12\x18\n" +
	"\amessage\x18\x01 \x01(\tR\amessage\x12\x1c\n" +
	"\timpending\x18\x02 \x01(\bR\timpending\x12$\n" +
	"\rmigrationLink\x18\x03 \x01(\tR\rmigrationLink\x12 \n" +
	"\vdescription\x18\x04 \x01(\tR\vdescription\x12,\n" +
	"\x11deprecatedVersion\x18\x05 \x01(\tR\x11deprecatedVersion\x12*\n" +
	"\x10scheduledVersion\x18\x06 \x01(\tR\x10scheduledVersion\")\n" +
	"\tStartedAt\x12\x1c\n" +
	"\tstartedAt\x18\x01 \x01(\x03R\tstartedAt\"?\n" +
	"\fOutboundList\x12/\n" +
	"\toutbounds\x18\x01 \x03(\v2\x11.daemon.GroupItemR\toutbounds\"\xb7\x01\n" +
	"\x19NetworkQualityTestRequest\x12\x1c\n" +
	"\tconfigURL\x18\x01 \x01(\tR\tconfigURL\x12 \n" +
	"\voutboundTag\x18\x02 \x01(\tR\voutboundTag\x12\x16\n" +
	"\x06serial\x18\x03 \x01(\bR\x06serial\x12,\n" +
	"\x11maxRuntimeSeconds\x18\x04 \x01(\x05R\x11maxRuntimeSeconds\x12\x14\n" +
	"\x05http3\x18\x05 \x01(\bR\x05http3\"\x8e\x04\n" +
	"\x1aNetworkQualityTestProgress\x12\x14\n" +
	"\x05phase\x18\x01 \x01(\x05R\x05phase\x12*\n" +
	"\x10downloadCapacity\x18\x02 \x01(\x03R\x10downloadCapacity\x12&\n" +
	"\x0euploadCapacity\x18\x03 \x01(\x03R\x0euploadCapacity\x12 \n" +
	"\vdownloadRPM\x18\x04 \x01(\x05R\vdownloadRPM\x12\x1c\n" +
	"\tuploadRPM\x18\x05 \x01(\x05R\tuploadRPM\x12$\n" +
	"\ridleLatencyMs\x18\x06 \x01(\x05R\ridleLatencyMs\x12\x1c\n" +
	"\telapsedMs\x18\a \x01(\x03R\telapsedMs\x12\x18\n" +
	"\aisFinal\x18\b \x01(\bR\aisFinal\x12\x14\n" +
	"\x05error\x18\t \x01(\tR\x05error\x12:\n" +
	"\x18downloadCapacityAccuracy\x18\n" +
	" \x01(\x05R\x18downloadCapacityAccuracy\x126\n" +
	"\x16uploadCapacityAccuracy\x18\v \x01(\x05R\x16uploadCapacityAccuracy\x120\n" +
	"\x13downloadRPMAccuracy\x18\f \x01(\x05R\x13downloadRPMAccuracy\x12,\n" +
	"\x11uploadRPMAccuracy\x18\r \x01(\x05R\x11uploadRPMAccuracy\"K\n" +
	"\x0fSTUNTestRequest\x12\x16\n" +
	"\x06server\x18\x01 \x01(\tR\x06server\x12 \n" +
	"\voutboundTag\x18\x02 \x01(\tR\voutboundTag\"\x8a\x02\n" +
	"\x10STUNTestProgress\x12\x14\n" +
	"\x05phase\x18\x01 \x01(\x05R\x05phase\x12\"\n" +
	"\fexternalAddr\x18\x02 \x01(\tR\fexternalAddr\x12\x1c\n" +
	"\tlatencyMs\x18\x03 \x01(\x05R\tlatencyMs\x12\x1e\n" +
	"\n" +
	"natMapping\x18\x04 \x01(\x05R\n" +
	"natMapping\x12\"\n" +
	"\fnatFiltering\x18\x05 \x01(\x05R\fnatFiltering\x12\x18\n" +
	"\aisFinal\x18\x06 \x01(\bR\aisFinal\x12\x14\n" +
	"\x05error\x18\a \x01(\tR\x05error\x12*\n" +
	"\x10natTypeSupported\x18\b \x01(\bR\x10natTypeSupported\"V\n" +
	"\x15TailscaleStatusUpdate\x12=\n" +
	"\tendpoints\x18\x01 \x03(\v2\x1f.daemon.TailscaleEndpointStatusR\tendpoints\"\xc1\x04\n" +
	"\x17TailscaleEndpointStatus\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\"\n" +
	"\fbackendState\x18\x02 \x01(\tR\fbackendState\x12\x1c\n" +
	"\tstateText\x18\x03 \x01(\tR\tstateText\x12\x18\n" +
	"\aauthURL\x18\x04 \x01(\tR\aauthURL\x12 \n" +
	"\vnetworkName\x18\x05 \x01(\tR\vnetworkName\x12&\n" +
	"\x0emagicDNSSuffix\x18\x06 \x01(\tR\x0emagicDNSSuffix\x12)\n" +
	"\x04self\x18\a \x01(\v2\x15.daemon.TailscalePeerR\x04self\x12:\n" +
	"\n" +
	"userGroups\x18\b \x03(\v2\x1a.daemon.TailscaleUserGroupR\n" +
	"userGroups\x121\n" +
	"\bexitNode\x18\t \x01(\v2\x15.daemon.TailscalePeerR\bexitNode\x12\x18\n" +
	"\akeyAuth\x18\n" +
	" \x01(\bR\akeyAuth\x12$\n" +
	"\rcanShareFiles\x18\v \x01(\bR\rcanShareFiles\x12*\n" +
	"\x10waitingFileCount\x18\f \x01(\x05R\x10waitingFileCount\x12.\n" +
	"\x12receivingFileCount\x18\r \x01(\x05R\x12receivingFileCount\x12(\n" +
	"\x0funreadFileCount\x18\x0e \x01(\x05R\x0funreadFileCount\"\xbf\x01\n" +
	"\x12TailscaleUserGroup\x12\x16\n" +
	"\x06userID\x18\x01 \x01(\x03R\x06userID\x12\x1c\n" +
	"\tloginName\x18\x02 \x01(\tR\tloginName\x12 \n" +
	"\vdisplayName\x18\x03 \x01(\tR\vdisplayName\x12$\n" +
	"\rprofilePicURL\x18\x04 \x01(\tR\rprofilePicURL\x12+\n" +
	"\x05peers\x18\x05 \x03(\v2\x15.daemon.TailscalePeerR\x05peers\"\xfd\x03\n" +
	"\rTailscalePeer\x12\x1a\n" +
	"\bhostName\x18\x01 \x01(\tR\bhostName\x12\x18\n" +
	"\adnsName\x18\x02 \x01(\tR\adnsName\x12\x0e\n" +
	"\x02os\x18\x03 \x01(\tR\x02os\x12\"\n" +
	"\ftailscaleIPs\x18\x04 \x03(\tR\ftailscaleIPs\x12\x16\n" +
	"\x06online\x18\x05 \x01(\bR\x06online\x12\x1a\n" +
	"\bexitNode\x18\x06 \x01(\bR\bexitNode\x12&\n" +
	"\x0eexitNodeOption\x18\a \x01(\bR\x0eexitNodeOption\x12\x16\n" +
	"\x06active\x18\b \x01(\bR\x06active\x12\x18\n" +
	"\arxBytes\x18\t \x01(\x03R\arxBytes\x12\x18\n" +
	"\atxBytes\x18\n" +
	" \x01(\x03R\atxBytes\x12\x1c\n" +
	"\tkeyExpiry\x18\v \x01(\x03R\tkeyExpiry\x12\x1a\n" +
	"\bstableID\x18\f \x01(\tR\bstableID\x12\x18\n" +
	"\aexpired\x18\r \x01(\bR\aexpired\x12 \n" +
	"\vsshHostKeys\x18\x0e \x03(\tR\vsshHostKeys\x12\x1e\n" +
	"\n" +
	"shareeNode\x18\x0f \x01(\bR\n" +
	"shareeNode\x12\x1a\n" +
	"\blastSeen\x18\x10 \x01(\x03R\blastSeen\x12(\n" +
	"\x0fcanReceiveFiles\x18\x11 \x01(\bR\x0fcanReceiveFiles\"P\n" +
	"\x14TailscalePingRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x16\n" +
	"\x06peerIP\x18\x02 \x01(\tR\x06peerIP\"\xed\x01\n" +
	"\x15TailscalePingResponse\x12\x1c\n" +
	"\tlatencyMs\x18\x01 \x01(\x01R\tlatencyMs\x12\x1a\n" +
	"\bisDirect\x18\x02 \x01(\bR\bisDirect\x12\x1a\n" +
	"\bendpoint\x18\x03 \x01(\tR\bendpoint\x12\"\n" +
	"\fderpRegionID\x18\x04 \x01(\x05R\fderpRegionID\x12&\n" +
	"\x0ederpRegionCode\x18\x05 \x01(\tR\x0ederpRegionCode\x12\x14\n" +
	"\x05error\x18\x06 \x01(\tR\x05error\x12\x1c\n" +
	"\tpeerRelay\x18\a \x01(\tR\tpeerRelay\"[\n" +
	"\x1bSetTailscaleExitNodeRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x1a\n" +
	"\bstableID\x18\x02 \x01(\tR\bstableID\":\n" +
	"\x16TailscaleLogoutRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\"\xc2\x01\n" +
	"\x19TailscaleSSHClientMessage\x121\n" +
	"\x05start\x18\x01 \x01(\v2\x19.daemon.TailscaleSSHStartH\x00R\x05start\x121\n" +
	"\x05input\x18\x02 \x01(\v2\x19.daemon.TailscaleSSHInputH\x00R\x05input\x124\n" +
	"\x06resize\x18\x03 \x01(\v2\x1a.daemon.TailscaleSSHResizeH\x00R\x06resizeB\t\n" +
	"\amessage\"\xcc\x02\n" +
	"\x11TailscaleSSHStart\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12 \n" +
	"\vpeerAddress\x18\x02 \x01(\tR\vpeerAddress\x12\x1a\n" +
	"\busername\x18\x03 \x01(\tR\busername\x12\"\n" +
	"\fterminalType\x18\x04 \x01(\tR\fterminalType\x12\x18\n" +
	"\acolumns\x18\x05 \x01(\x05R\acolumns\x12\x12\n" +
	"\x04rows\x18\x06 \x01(\x05R\x04rows\x12 \n" +
	"\vwidthPixels\x18\a \x01(\x05R\vwidthPixels\x12\"\n" +
	"\fheightPixels\x18\b \x01(\x05R\fheightPixels\x12\x1a\n" +
	"\bhostKeys\x18\t \x03(\tR\bhostKeys\x12#\n" +
	"\rforward_agent\x18\n" +
	" \x01(\bR\fforwardAgent\"'\n" +
	"\x11TailscaleSSHInput\x12\x12\n" +
	"\x04data\x18\x01 \x01(\fR\x04data\"\x88\x01\n" +
	"\x12TailscaleSSHResize\x12\x18\n" +
	"\acolumns\x18\x01 \x01(\x05R\acolumns\x12\x12\n" +
	"\x04rows\x18\x02 \x01(\x05R\x04rows\x12 \n" +
	"\vwidthPixels\x18\x03 \x01(\x05R\vwidthPixels\x12\"\n" +
	"\fheightPixels\x18\x04 \x01(\x05R\fheightPixels\"\xb4\x02\n" +
	"\x19TailscaleSSHServerMessage\x12@\n" +
	"\n" +
	"authBanner\x18\x01 \x01(\v2\x1e.daemon.TailscaleSSHAuthBannerH\x00R\n" +
	"authBanner\x121\n" +
	"\x05ready\x18\x02 \x01(\v2\x19.daemon.TailscaleSSHReadyH\x00R\x05ready\x124\n" +
	"\x06output\x18\x03 \x01(\v2\x1a.daemon.TailscaleSSHOutputH\x00R\x06output\x12.\n" +
	"\x04exit\x18\x04 \x01(\v2\x18.daemon.TailscaleSSHExitH\x00R\x04exit\x121\n" +
	"\x05error\x18\x05 \x01(\v2\x19.daemon.TailscaleSSHErrorH\x00R\x05errorB\t\n" +
	"\amessage\"2\n" +
	"\x16TailscaleSSHAuthBanner\x12\x18\n" +
	"\amessage\x18\x01 \x01(\tR\amessage\"\x13\n" +
	"\x11TailscaleSSHReady\"(\n" +
	"\x12TailscaleSSHOutput\x12\x12\n" +
	"\x04data\x18\x01 \x01(\fR\x04data\"j\n" +
	"\x10TailscaleSSHExit\x12\x1a\n" +
	"\bexitCode\x18\x01 \x01(\x05R\bexitCode\x12\x16\n" +
	"\x06signal\x18\x02 \x01(\tR\x06signal\x12\"\n" +
	"\ferrorMessage\x18\x03 \x01(\tR\ferrorMessage\"-\n" +
	"\x11TailscaleSSHError\x12\x18\n" +
	"\amessage\x18\x01 \x01(\tR\amessage\"A\n" +
	"\x1dSubscribeTaildropInboxRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\"@\n" +
	"\x1cMarkTaildropInboxReadRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\"\x9a\x01\n" +
	"\rTaildropInbox\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12*\n" +
	"\x05files\x18\x02 \x03(\v2\x14.daemon.TaildropFileR\x05files\x12;\n" +
	"\treceiving\x18\x03 \x03(\v2\x1d.daemon.TaildropReceivingFileR\treceiving\"v\n" +
	"\fTaildropFile\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x12\n" +
	"\x04size\x18\x02 \x01(\x03R\x04size\x12\x1e\n" +
	"\n" +
	"senderName\x18\x03 \x01(\tR\n" +
	"senderName\x12\x1e\n" +
	"\n" +
	"modifiedAt\x18\x04 \x01(\x03R\n" +
	"modifiedAt\"\xa1\x01\n" +
	"\x15TaildropReceivingFile\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x12\n" +
	"\x04size\x18\x02 \x01(\x03R\x04size\x12$\n" +
	"\rreceivedBytes\x18\x03 \x01(\x03R\rreceivedBytes\x12\x1a\n" +
	"\bsenderID\x18\x04 \x01(\tR\bsenderID\x12\x1e\n" +
	"\n" +
	"senderName\x18\x05 \x01(\tR\n" +
	"senderName\"\xc4\x01\n" +
	"\x19TaildropSendClientMessage\x121\n" +
	"\x05start\x18\x01 \x01(\v2\x19.daemon.TaildropSendStartH\x00R\x05start\x121\n" +
	"\x05chunk\x18\x02 \x01(\v2\x19.daemon.TaildropFileChunkH\x00R\x05chunk\x126\n" +
	"\bfileDone\x18\x03 \x01(\v2\x18.daemon.TaildropFileDoneH\x00R\bfileDoneB\t\n" +
	"\amessage\"\x8d\x01\n" +
	"\x11TaildropSendStart\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\"\n" +
	"\fpeerStableID\x18\x02 \x01(\tR\fpeerStableID\x122\n" +
	"\x05files\x18\x03 \x03(\v2\x1c.daemon.TaildropOutgoingFileR\x05files\">\n" +
	"\x14TaildropOutgoingFile\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x12\n" +
	"\x04size\x18\x02 \x01(\x03R\x04size\"'\n" +
	"\x11TaildropFileChunk\x12\x12\n" +
	"\x04data\x18\x01 \x01(\fR\x04data\"\x12\n" +
	"\x10TaildropFileDone\"\x8a\x01\n" +
	"\x19TaildropSendServerMessage\x12:\n" +
	"\bprogress\x18\x01 \x01(\v2\x1c.daemon.TaildropSendProgressH\x00R\bprogress\x12&\n" +
	"\rreceivedBytes\x18\x02 \x01(\x03H\x00R\rreceivedBytesB\t\n" +
	"\amessage\"x\n" +
	"\x14TaildropSendProgress\x12\x1c\n" +
	"\tfileIndex\x18\x01 \x01(\x05R\tfileIndex\x12\x1c\n" +
	"\tsentBytes\x18\x02 \x01(\x03R\tsentBytes\x12$\n" +
	"\rfileCompleted\x18\x03 \x01(\bR\rfileCompleted\"S\n" +
	"\x1bDownloadTaildropFileRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x12\n" +
	"\x04name\x18\x02 \x01(\tR\x04name\"C\n" +
	"\x19DownloadTaildropFileChunk\x12\x12\n" +
	"\x04size\x18\x01 \x01(\x03R\x04size\x12\x12\n" +
	"\x04data\x18\x02 \x01(\fR\x04data\"Q\n" +
	"\x19DeleteTaildropFileRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x12\n" +
	"\x04name\x18\x02 \x01(\tR\x04name\"r\n" +
	"\x1eCancelTaildropReceivingRequest\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x1a\n" +
	"\bsenderID\x18\x02 \x01(\tR\bsenderID\x12\x12\n" +
	"\x04name\x18\x03 \x01(\tR\x04name\"\xc1\x01\n" +
	"\x12USBProviderMessage\x121\n" +
	"\x06attach\x18\x01 \x01(\v2\x17.daemon.USBDeviceAttachH\x00R\x06attach\x121\n" +
	"\x06detach\x18\x02 \x01(\v2\x17.daemon.USBDeviceDetachH\x00R\x06detach\x12:\n" +
	"\vurbResponse\x18\x03 \x01(\v2\x16.daemon.USBURBResponseH\x00R\vurbResponseB\t\n" +
	"\amessage\"\xe2\x01\n" +
	"\x10USBServerMessage\x12.\n" +
	"\x05ready\x18\x01 \x01(\v2\x16.daemon.USBDeviceReadyH\x00R\x05ready\x127\n" +
	"\n" +
	"urbRequest\x18\x02 \x01(\v2\x15.daemon.USBURBRequestH\x00R\n" +
	"urbRequest\x120\n" +
	"\x05abort\x18\x03 \x01(\v2\x18.daemon.USBEndpointAbortH\x00R\x05abort\x12(\n" +
	"\x05error\x18\x04 \x01(\v2\x10.daemon.USBErrorH\x00R\x05errorB\t\n" +
	"\amessage\"\x87\x04\n" +
	"\x13USBDeviceDescriptor\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x16\n" +
	"\x06busNum\x18\x02 \x01(\rR\x06busNum\x12\x16\n" +
	"\x06devNum\x18\x03 \x01(\rR\x06devNum\x12\x14\n" +
	"\x05speed\x18\x04 \x01(\rR\x05speed\x12\x1a\n" +
	"\bvendorId\x18\x05 \x01(\rR\bvendorId\x12\x1c\n" +
	"\tproductId\x18\x06 \x01(\rR\tproductId\x12\x1c\n" +
	"\tbcdDevice\x18\a \x01(\rR\tbcdDevice\x12 \n" +
	"\vdeviceClass\x18\b \x01(\rR\vdeviceClass\x12&\n" +
	"\x0edeviceSubClass\x18\t \x01(\rR\x0edeviceSubClass\x12&\n" +
	"\x0edeviceProtocol\x18\n" +
	" \x01(\rR\x0edeviceProtocol\x12.\n" +
	"\x12configurationValue\x18\v \x01(\rR\x12configurationValue\x12,\n" +
	"\x11numConfigurations\x18\f \x01(\rR\x11numConfigurations\x124\n" +
	"\n" +
	"interfaces\x18\r \x03(\v2\x14.daemon.USBInterfaceR\n" +
	"interfaces\x12\x16\n" +
	"\x06serial\x18\x0e \x01(\tR\x06serial\x12\x18\n" +
	"\aproduct\x18\x0f \x01(\tR\aproduct\"l\n" +
	"\x0fUSBDeviceAttach\x12\x1c\n" +
	"\tserverTag\x18\x01 \x01(\tR\tserverTag\x12;\n" +
	"\n" +
	"descriptor\x18\x02 \x01(\v2\x1b.daemon.USBDeviceDescriptorR\n" +
	"descriptor\"\x92\x01\n" +
	"\fUSBInterface\x12&\n" +
	"\x0einterfaceClass\x18\x01 \x01(\rR\x0einterfaceClass\x12,\n" +
	"\x11interfaceSubClass\x18\x02 \x01(\rR\x11interfaceSubClass\x12,\n" +
	"\x11interfaceProtocol\x18\x03 \x01(\rR\x11interfaceProtocol\"-\n" +
	"\x0fUSBDeviceDetach\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\"B\n" +
	"\x0eUSBDeviceReady\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x14\n" +
	"\x05busId\x18\x02 \x01(\tR\x05busId\"\xa1\x03\n" +
	"\rUSBURBRequest\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x10\n" +
	"\x03seq\x18\x02 \x01(\x04R\x03seq\x12\x1a\n" +
	"\bendpoint\x18\x03 \x01(\rR\bendpoint\x12 \n" +
	"\vdirectionIn\x18\x04 \x01(\bR\vdirectionIn\x12$\n" +
	"\rtransferFlags\x18\x05 \x01(\rR\rtransferFlags\x12\x14\n" +
	"\x05setup\x18\x06 \x01(\fR\x05setup\x122\n" +
	"\x14transferBufferLength\x18\a \x01(\rR\x14transferBufferLength\x12\x18\n" +
	"\aoutData\x18\b \x01(\fR\aoutData\x12(\n" +
	"\x0fnumberOfPackets\x18\t \x01(\x05R\x0fnumberOfPackets\x12\x1e\n" +
	"\n" +
	"startFrame\x18\n" +
	" \x01(\x05R\n" +
	"startFrame\x12\x1a\n" +
	"\binterval\x18\v \x01(\x05R\binterval\x124\n" +
	"\n" +
	"isoPackets\x18\f \x03(\v2\x14.daemon.USBIsoPacketR\n" +
	"isoPackets\"\xc8\x01\n" +
	"\x0eUSBURBResponse\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x10\n" +
	"\x03seq\x18\x02 \x01(\x04R\x03seq\x12\x16\n" +
	"\x06status\x18\x03 \x01(\x05R\x06status\x12\"\n" +
	"\factualLength\x18\x04 \x01(\x05R\factualLength\x12\x16\n" +
	"\x06inData\x18\x05 \x01(\fR\x06inData\x124\n" +
	"\n" +
	"isoPackets\x18\x06 \x03(\v2\x14.daemon.USBIsoPacketR\n" +
	"isoPackets\"z\n" +
	"\fUSBIsoPacket\x12\x16\n" +
	"\x06offset\x18\x01 \x01(\x05R\x06offset\x12\x16\n" +
	"\x06length\x18\x02 \x01(\x05R\x06length\x12\"\n" +
	"\factualLength\x18\x03 \x01(\x05R\factualLength\x12\x16\n" +
	"\x06status\x18\x04 \x01(\x05R\x06status\"J\n" +
	"\x10USBEndpointAbort\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x1a\n" +
	"\bendpoint\x18\x02 \x01(\rR\bendpoint\"@\n" +
	"\bUSBError\x12\x1a\n" +
	"\bdeviceId\x18\x01 \x01(\tR\bdeviceId\x12\x18\n" +
	"\amessage\x18\x02 \x01(\tR\amessage\"N\n" +
	"\x17USBIPServerStatusUpdate\x123\n" +
	"\aservers\x18\x01 \x03(\v2\x19.daemon.USBIPServerStatusR\aservers\"d\n" +
	"\x11USBIPServerStatus\x12\x1c\n" +
	"\tserverTag\x18\x01 \x01(\tR\tserverTag\x121\n" +
	"\adevices\x18\x02 \x03(\v2\x17.daemon.USBSharedDeviceR\adevices\"\xdc\x01\n" +
	"\x0fUSBSharedDevice\x12;\n" +
	"\n" +
	"descriptor\x18\x01 \x01(\v2\x1b.daemon.USBDeviceDescriptorR\n" +
	"descriptor\x12\x14\n" +
	"\x05busId\x18\x02 \x01(\tR\x05busId\x12\x1a\n" +
	"\bstableId\x18\x03 \x01(\tR\bstableId\x12,\n" +
	"\abackend\x18\x04 \x01(\x0e2\x12.daemon.USBBackendR\abackend\x12,\n" +
	"\x05state\x18\x05 \x01(\x0e2\x16.daemon.USBDeviceStateR\x05state\"Z\n" +
	"\x17OpenConnectStatusUpdate\x12?\n" +
	"\tendpoints\x18\x01 \x03(\v2!.daemon.OpenConnectEndpointStatusR\tendpoints\"\x8e\x02\n" +
	"\x19OpenConnectEndpointStatus\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x14\n" +
	"\x05state\x18\x02 \x01(\tR\x05state\x12\x1c\n" +
	"\tstateText\x18\x03 \x01(\tR\tstateText\x12F\n" +
	"\rauthChallenge\x18\x04 \x01(\v2 .daemon.OpenConnectAuthChallengeR\rauthChallenge\x12\x14\n" +
	"\x05error\x18\x05 \x01(\tR\x05error\x12=\n" +
	"\n" +
	"tunnelInfo\x18\x06 \x01(\v2\x1d.daemon.OpenConnectTunnelInfoR\n" +
	"tunnelInfo\"\xd9\x01\n" +
	"\x15OpenConnectTunnelInfo\x12\x16\n" +
	"\x06server\x18\x01 \x01(\tR\x06server\x12\x16\n" +
	"\x06flavor\x18\x02 \x01(\tR\x06flavor\x12\x1c\n" +
	"\ttransport\x18\x03 \x01(\tR\ttransport\x12\x12\n" +
	"\x04ipv4\x18\x04 \x03(\tR\x04ipv4\x12\x12\n" +
	"\x04ipv6\x18\x05 \x03(\tR\x04ipv6\x12\x10\n" +
	"\x03dns\x18\x06 \x03(\tR\x03dns\x12\x10\n" +
	"\x03mtu\x18\a \x01(\rR\x03mtu\x12&\n" +
	"\x0econnectedSince\x18\b \x01(\x03R\x0econnectedSince\"\xf1\x01\n" +
	"\x18OpenConnectAuthChallenge\x12\x0e\n" +
	"\x02id\x18\x01 \x01(\tR\x02id\x12\x16\n" +
	"\x06banner\x18\x02 \x01(\tR\x06banner\x12\x18\n" +
	"\amessage\x18\x03 \x01(\tR\amessage\x12\x14\n" +
	"\x05error\x18\x04 \x01(\tR\x05error\x121\n" +
	"\x04form\x18\x05 \x01(\v2\x1b.daemon.OpenConnectAuthFormH\x00R\x04form\x12=\n" +
	"\abrowser\x18\x06 \x01(\v2!.daemon.OpenConnectBrowserRequestH\x00R\abrowserB\v\n" +
	"\tchallenge\"O\n" +
	"\x13OpenConnectAuthForm\x128\n" +
	"\x06fields\x18\x01 \x03(\v2 .daemon.OpenConnectAuthFormFieldR\x06fields\"\xd1\x01\n" +
	"\x18OpenConnectAuthFormField\x12$\n" +
	"\rsubmissionKey\x18\x01 \x01(\tR\rsubmissionKey\x12\x12\n" +
	"\x04name\x18\x02 \x01(\tR\x04name\x12\x14\n" +
	"\x05label\x18\x03 \x01(\tR\x05label\x12\x12\n" +
	"\x04kind\x18\x04 \x01(\tR\x04kind\x12\x14\n" +
	"\x05value\x18\x05 \x01(\tR\x05value\x12;\n" +
	"\aoptions\x18\x06 \x03(\v2!.daemon.OpenConnectAuthFormChoiceR\aoptions\"G\n" +
	"\x19OpenConnectAuthFormChoice\x12\x14\n" +
	"\x05value\x18\x01 \x01(\tR\x05value\x12\x14\n" +
	"\x05label\x18\x02 \x01(\tR\x05label\"\x85\x02\n" +
	"\x19OpenConnectBrowserRequest\x12\x10\n" +
	"\x03url\x18\x01 \x01(\tR\x03url\x12\x1a\n" +
	"\bfinalURL\x18\x02 \x01(\tR\bfinalURL\x12 \n" +
	"\vcookieNames\x18\x03 \x03(\tR\vcookieNames\x12 \n" +
	"\vheaderNames\x18\x04 \x03(\tR\vheaderNames\x120\n" +
	"\x13callbackURLPrefixes\x18\x05 \x03(\tR\x13callbackURLPrefixes\x12*\n" +
	"\x10earlyCookieNames\x18\x06 \x03(\tR\x10earlyCookieNames\x12\x18\n" +
	"\acacheID\x18\a \x01(\tR\acacheID\"D\n" +
	"\x18OpenConnectBrowserCookie\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x14\n" +
	"\x05value\x18\x02 \x01(\tR\x05value\"F\n" +
	"\x18OpenConnectBrowserHeader\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x16\n" +
	"\x06values\x18\x02 \x03(\tR\x06values\"\xa1\x01\n" +
	"\x1bOpenConnectAuthFormResponse\x12G\n" +
	"\x06values\x18\x01 \x03(\v2/.daemon.OpenConnectAuthFormResponse.ValuesEntryR\x06values\x1a9\n" +
	"\vValuesEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n" +
	"\x05value\x18\x02 \x01(\tR\x05value:\x028\x01\"\xae\x01\n" +
	"\x18OpenConnectBrowserResult\x12\x1a\n" +
	"\bfinalURL\x18\x01 \x01(\tR\bfinalURL\x12:\n" +
	"\acookies\x18\x02 \x03(\v2 .daemon.OpenConnectBrowserCookieR\acookies\x12:\n" +
	"\aheaders\x18\x03 \x03(\v2 .daemon.OpenConnectBrowserHeaderR\aheaders\"\xec\x01\n" +
	"!OpenConnectAuthResponseSubmission\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12 \n" +
	"\vchallengeID\x18\x02 \x01(\tR\vchallengeID\x129\n" +
	"\x04form\x18\x03 \x01(\v2#.daemon.OpenConnectAuthFormResponseH\x00R\x04form\x12<\n" +
	"\abrowser\x18\x04 \x01(\v2 .daemon.OpenConnectBrowserResultH\x00R\abrowserB\n" +
	"\n" +
	"\bresponse\"d\n" +
	"\x1eOpenConnectAuthChallengeCancel\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12 \n" +
	"\vchallengeID\x18\x02 \x01(\tR\vchallengeID\"R\n" +
	"\x13OpenVPNStatusUpdate\x12;\n" +
	"\tendpoints\x18\x01 \x03(\v2\x1d.daemon.OpenVPNEndpointStatusR\tendpoints\"\xf6\x01\n" +
	"\x15OpenVPNEndpointStatus\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12\x14\n" +
	"\x05state\x18\x02 \x01(\tR\x05state\x12\x1c\n" +
	"\tstateText\x18\x03 \x01(\tR\tstateText\x126\n" +
	"\tchallenge\x18\x04 \x01(\v2\x18.daemon.OpenVPNChallengeR\tchallenge\x12\x14\n" +
	"\x05error\x18\x05 \x01(\tR\x05error\x129\n" +
	"\n" +
	"tunnelInfo\x18\x06 \x01(\v2\x19.daemon.OpenVPNTunnelInfoR\n" +
	"tunnelInfo\"\xd7\x01\n" +
	"\x11OpenVPNTunnelInfo\x12\x16\n" +
	"\x06server\x18\x01 \x01(\tR\x06server\x12\x18\n" +
	"\anetwork\x18\x03 \x01(\tR\anetwork\x12\x12\n" +
	"\x04ipv4\x18\x04 \x03(\tR\x04ipv4\x12\x12\n" +
	"\x04ipv6\x18\x05 \x03(\tR\x04ipv6\x12\x10\n" +
	"\x03dns\x18\x06 \x03(\tR\x03dns\x12\x10\n" +
	"\x03mtu\x18\a \x01(\rR\x03mtu\x12&\n" +
	"\x0econnectedSince\x18\b \x01(\x03R\x0econnectedSince\x12\x16\n" +
	"\x06cipher\x18\t \x01(\tR\x06cipherJ\x04\b\x02\x10\x03\"\xfa\x01\n" +
	"\x10OpenVPNChallenge\x12\x0e\n" +
	"\x02id\x18\x01 \x01(\tR\x02id\x12\x12\n" +
	"\x04kind\x18\x02 \x01(\tR\x04kind\x12\x1a\n" +
	"\busername\x18\x03 \x01(\tR\busername\x12\x18\n" +
	"\amessage\x18\x04 \x01(\tR\amessage\x12\x10\n" +
	"\x03url\x18\x05 \x01(\tR\x03url\x12$\n" +
	"\rsecretMessage\x18\x06 \x01(\tR\rsecretMessage\x12\x12\n" +
	"\x04echo\x18\a \x01(\bR\x04echo\x12$\n" +
	"\rpreviousError\x18\b \x01(\tR\rpreviousError\x12\x1a\n" +
	"\bdeadline\x18\t \x01(\x03R\bdeadline\"\xb0\x01\n" +
	"\x1aOpenVPNChallengeSubmission\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12 \n" +
	"\vchallengeID\x18\x02 \x01(\tR\vchallengeID\x12\x1a\n" +
	"\busername\x18\x03 \x01(\tR\busername\x12\x1a\n" +
	"\bpassword\x18\x04 \x01(\tR\bpassword\x12\x16\n" +
	"\x06secret\x18\x05 \x01(\tR\x06secret\"\\\n" +
	"\x16OpenVPNChallengeCancel\x12 \n" +
	"\vendpointTag\x18\x01 \x01(\tR\vendpointTag\x12 \n" +
	"\vchallengeID\x18\x02 \x01(\tR\vchallengeID\"~\n" +
	"\x11NotificationEvent\x12*\n" +
	"\x04send\x18\x01 \x01(\v2\x14.daemon.NotificationH\x00R\x04send\x124\n" +
	"\x06cancel\x18\x02 \x01(\v2\x1a.daemon.NotificationCancelH\x00R\x06cancelB\a\n" +
	"\x05event\"\xc2\x01\n" +
	"\fNotification\x12\x1e\n" +
	"\n" +
	"identifier\x18\x01 \x01(\tR\n" +
	"identifier\x12\x1a\n" +
	"\btypeName\x18\x02 \x01(\tR\btypeName\x12\x16\n" +
	"\x06typeID\x18\x03 \x01(\x05R\x06typeID\x12\x14\n" +
	"\x05title\x18\x04 \x01(\tR\x05title\x12\x1a\n" +
	"\bsubtitle\x18\x05 \x01(\tR\bsubtitle\x12\x12\n" +
	"\x04body\x18\x06 \x01(\tR\x04body\x12\x18\n" +
	"\aopenURL\x18\a \x01(\tR\aopenURL\"L\n" +
	"\x12NotificationCancel\x12\x1e\n" +
	"\n" +
	"identifier\x18\x01 \x01(\tR\n" +
	"identifier\x12\x16\n" +
	"\x06typeID\x18\x02 \x01(\x05R\x06typeID\"h\n" +
	"\x16URLTestOutboundRequest\x12 \n" +
	"\voutboundTag\x18\x01 \x01(\tR\voutboundTag\x12\x12\n" +
	"\x04link\x18\x02 \x01(\tR\x04link\x12\x18\n" +
	"\atimeout\x18\x03 \x01(\rR\atimeout\"E\n" +
	"\x17URLTestOutboundResponse\x12\x14\n" +
	"\x05delay\x18\x01 \x01(\rR\x05delay\x12\x14\n" +
	"\x05error\x18\x02 \x01(\tR\x05error\"8\n" +
	"\x0eHttpHeaderPair\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n" +
	"\x05value\x18\x02 \x01(\tR\x05value\"\xb8\x01\n" +
	"\x18GetURLViaOutboundRequest\x12 \n" +
	"\voutboundTag\x18\x01 \x01(\tR\voutboundTag\x12\x12\n" +
	"\x04link\x18\x02 \x01(\tR\x04link\x12\x18\n" +
	"\atimeout\x18\x03 \x01(\rR\atimeout\x12\x1a\n" +
	"\bmaxBytes\x18\x04 \x01(\rR\bmaxBytes\x120\n" +
	"\aheaders\x18\x05 \x03(\v2\x16.daemon.HttpHeaderPairR\aheaders\"\xe3\x01\n" +
	"\x19GetURLViaOutboundResponse\x12\x1e\n" +
	"\n" +
	"httpStatus\x18\x01 \x01(\rR\n" +
	"httpStatus\x12\x12\n" +
	"\x04body\x18\x02 \x01(\fR\x04body\x12\x1c\n" +
	"\ttruncated\x18\x03 \x01(\bR\ttruncated\x12 \n" +
	"\vcontentType\x18\x04 \x01(\tR\vcontentType\x12\x1e\n" +
	"\n" +
	"remoteAddr\x18\x05 \x01(\tR\n" +
	"remoteAddr\x12\x1c\n" +
	"\telapsedMs\x18\x06 \x01(\rR\telapsedMs\x12\x14\n" +
	"\x05error\x18\a \x01(\tR\x05error\"b\n" +
	"\x04Rule\x12\x12\n" +
	"\x04type\x18\x01 \x01(\tR\x04type\x12\x18\n" +
	"\apayload\x18\x02 \x01(\tR\apayload\x12\x16\n" +
	"\x06action\x18\x03 \x01(\tR\x06action\x12\x14\n" +
	"\x05isDNS\x18\x04 \x01(\bR\x05isDNS\".\n" +
	"\bRuleList\x12\"\n" +
	"\x05rules\x18\x01 \x03(\v2\f.daemon.RuleR\x05rules\"D\n" +
	"\x1aSubscribeDNSQueriesRequest\x12&\n" +
	"\x0eincludeAnswers\x18\x01 \x01(\bR\x0eincludeAnswers\"\x84\x04\n" +
	"\rDnsQueryEvent\x12\x16\n" +
	"\x06domain\x18\x01 \x01(\tR\x06domain\x12\x1c\n" +
	"\tqueryType\x18\x02 \x01(\rR\tqueryType\x12\x14\n" +
	"\x05rcode\x18\x03 \x01(\x05R\x05rcode\x12\x10\n" +
	"\x03ttl\x18\x04 \x01(\rR\x03ttl\x12\x16\n" +
	"\x06source\x18\x05 \x01(\tR\x06source\x125\n" +
	"\vprocessInfo\x18\x06 \x01(\v2\x13.daemon.ProcessInfoR\vprocessInfo\x12\x16\n" +
	"\x06failed\x18\a \x01(\bR\x06failed\x12\x14\n" +
	"\x05error\x18\b \x01(\tR\x05error\x12+\n" +
	"\aanswers\x18\t \x03(\v2\x11.daemon.DnsAnswerR\aanswers\x12\x1c\n" +
	"\tdnsServer\x18\n" +
	" \x01(\tR\tdnsServer\x12$\n" +
	"\rdnsServerType\x18\v \x01(\tR\rdnsServerType\x12\x1a\n" +
	"\boutbound\x18\f \x03(\tR\boutbound\x12\"\n" +
	"\fdnsGroupPath\x18\r \x03(\tR\fdnsGroupPath\x123\n" +
	"\battempts\x18\x0e \x03(\v2\x17.daemon.DnsGroupAttemptR\battempts\x12\x16\n" +
	"\x06fanned\x18\x0f \x01(\bR\x06fanned\x12\x1a\n" +
	"\bsurvival\x18\x10 \x01(\bR\bsurvival\"y\n" +
	"\x0fDnsGroupAttempt\x12\x16\n" +
	"\x06server\x18\x01 \x01(\tR\x06server\x12\x1e\n" +
	"\n" +
	"serverType\x18\x02 \x01(\tR\n" +
	"serverType\x12\x18\n" +
	"\aoutcome\x18\x03 \x01(\tR\aoutcome\x12\x14\n" +
	"\x05rttMs\x18\x04 \x01(\rR\x05rttMs\"[\n" +
	"\tDnsAnswer\x12\x12\n" +
	"\x04name\x18\x01 \x01(\tR\x04name\x12\x12\n" +
	"\x04type\x18\x02 \x01(\rR\x04type\x12\x14\n" +
	"\x05rdata\x18\x03 \x01(\tR\x05rdata\x12\x10\n" +
	"\x03ttl\x18\x04 \x01(\rR\x03ttl\",\n" +
	"\x0eGetPoolRequest\x12\x1a\n" +
	"\bgroupTag\x18\x01 \x01(\tR\bgroupTag\"F\n" +
	"\bPoolSlot\x12\x12\n" +
	"\x04slot\x18\x01 \x01(\rR\x04slot\x12\x10\n" +
	"\x03tag\x18\x02 \x01(\tR\x03tag\x12\x14\n" +
	"\x05delay\x18\x03 \x01(\rR\x05delay\"2\n" +
	"\bPoolList\x12&\n" +
	"\x05slots\x18\x01 \x03(\v2\x10.daemon.PoolSlotR\x05slots\"\xf4\x01\n" +
	"\x0eDnsGroupMember\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x12\x1e\n" +
	"\n" +
	"serverType\x18\x02 \x01(\tR\n" +
	"serverType\x12\x14\n" +
	"\x05clean\x18\x03 \x01(\bR\x05clean\x12\x1e\n" +
	"\n" +
	"liveErrors\x18\x04 \x01(\rR\n" +
	"liveErrors\x12&\n" +
	"\x0elastErrorAgeMs\x18\x05 \x01(\x03R\x0elastErrorAgeMs\x12\x1a\n" +
	"\bliveWins\x18\x06 \x01(\rR\bliveWins\x12\x18\n" +
	"\acurrent\x18\a \x01(\bR\acurrent\x12\x1c\n" +
	"\tlastRttMs\x18\b \x01(\rR\tlastRttMs\"\x81\x01\n" +
	"\rDnsGroupState\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x12\x12\n" +
	"\x04mode\x18\x02 \x01(\tR\x04mode\x12\x18\n" +
	"\acurrent\x18\x03 \x01(\tR\acurrent\x120\n" +
	"\amembers\x18\x06 \x03(\v2\x16.daemon.DnsGroupMemberR\amembers\"=\n" +
	"\fDnsGroupList\x12-\n" +
	"\x06groups\x18\x01 \x03(\v2\x15.daemon.DnsGroupStateR\x06groups\")\n" +
	"\rRunningConfig\x12\x18\n" +
	"\acontent\x18\x01 \x01(\tR\acontent\"\xd7\x02\n" +
	"\x0fChainCloneState\x12\x14\n" +
	"\x05state\x18\x01 \x01(\tR\x05state\x12 \n" +
	"\vactiveConns\x18\x02 \x01(\x03R\vactiveConns\x12(\n" +
	"\x0flastPickedAgeMs\x18\x03 \x01(\x03R\x0flastPickedAgeMs\x12\"\n" +
	"\fcreatedAgeMs\x18\x04 \x01(\x03R\fcreatedAgeMs\x12$\n" +
	"\rmtuConfigured\x18\x05 \x01(\rR\rmtuConfigured\x12\"\n" +
	"\fmtuEffective\x18\x06 \x01(\rR\fmtuEffective\x12\x1c\n" +
	"\tmtuReason\x18\a \x01(\tR\tmtuReason\x12\x1a\n" +
	"\bstripped\x18\b \x03(\tR\bstripped\x12\x1c\n" +
	"\trewritten\x18\t \x01(\bR\trewritten\x12\x1c\n" +
	"\tlastError\x18\n" +
	" \x01(\tR\tlastError\"\xd2\x01\n" +
	"\rChainPosition\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x12\x18\n" +
	"\aisGroup\x18\x02 \x01(\bR\aisGroup\x12\x10\n" +
	"\x03now\x18\x03 \x01(\tR\x03now\x12 \n" +
	"\vtransparent\x18\x04 \x01(\bR\vtransparent\x12\x16\n" +
	"\x06errors\x18\x05 \x01(\x03R\x06errors\x12-\n" +
	"\x05clone\x18\x06 \x01(\v2\x17.daemon.ChainCloneStateR\x05clone\x12\x1a\n" +
	"\bdisabled\x18\a \x01(\bR\bdisabled\"\xed\x01\n" +
	"\n" +
	"ChainState\x12\x10\n" +
	"\x03tag\x18\x01 \x01(\tR\x03tag\x123\n" +
	"\tpositions\x18\x02 \x03(\v2\x15.daemon.ChainPositionR\tpositions\x12\x14\n" +
	"\x05dials\x18\x03 \x01(\x03R\x05dials\x12\x16\n" +
	"\x06errors\x18\x04 \x01(\x03R\x06errors\x12$\n" +
	"\rclonesCreated\x18\x05 \x01(\x03R\rclonesCreated\x12$\n" +
	"\rclonesEvicted\x18\x06 \x01(\x03R\rclonesEvicted\x12\x1e\n" +
	"\n" +
	"liveClones\x18\a \x01(\x03R\n" +
	"liveClones\"7\n" +
	"\tChainList\x12*\n" +
	"\x06chains\x18\x01 \x03(\v2\x12.daemon.ChainStateR\x06chains\"r\n" +
	"\x1eSetChainPositionEnabledRequest\x12\x1a\n" +
	"\bchainTag\x18\x01 \x01(\tR\bchainTag\x12\x1a\n" +
	"\bposition\x18\x02 \x01(\x05R\bposition\x12\x18\n" +
	"\aenabled\x18\x03 \x01(\bR\aenabled\"C\n" +
	"\x1fSetChainPositionEnabledResponse\x12 \n" +
	"\vwarmupError\x18\x01 \x01(\tR\vwarmupError\"T\n" +
	"\x1aGetChainCloneConfigRequest\x12\x1a\n" +
	"\bchainTag\x18\x01 \x01(\tR\bchainTag\x12\x1a\n" +
	"\bposition\x18\x02 \x01(\x05R\bposition*U\n" +
	"\bLogLevel\x12\t\n" +
	"\x05PANIC\x10\x00\x12\t\n" +
	"\x05FATAL\x10\x01\x12\t\n" +
	"\x05ERROR\x10\x02\x12\b\n" +
	"\x04WARN\x10\x03\x12\b\n" +
	"\x04INFO\x10\x04\x12\t\n" +
	"\x05DEBUG\x10\x05\x12\t\n" +
	"\x05TRACE\x10\x06*i\n" +
	"\x13ConnectionEventType\x12\x18\n" +
	"\x14CONNECTION_EVENT_NEW\x10\x00\x12\x1b\n" +
	"\x17CONNECTION_EVENT_UPDATE\x10\x01\x12\x1b\n" +
	"\x17CONNECTION_EVENT_CLOSED\x10\x02*l\n" +
	"\x0eUSBDeviceState\x12\x19\n" +
	"\x15USB_DEVICE_STATE_IDLE\x10\x00\x12\x1d\n" +
	"\x19USB_DEVICE_STATE_ATTACHED\x10\x01\x12 \n" +
	"\x1cUSB_DEVICE_STATE_UNAVAILABLE\x10\x02*\x9e\x01\n" +
	"\n" +
	"USBBackend\x12\x1b\n" +
	"\x17USB_BACKEND_UNSPECIFIED\x10\x00\x12\x1b\n" +
	"\x17USB_BACKEND_LINUX_SYSFS\x10\x01\x12\x17\n" +
	"\x13USB_BACKEND_DYNAMIC\x10\x02\x12\x1c\n" +
	"\x18USB_BACKEND_DARWIN_IOKIT\x10\x03\x12\x1f\n" +
	"\x1bUSB_BACKEND_WINDOWS_VBOXUSB\x10\x042\xea \n" +
	"\x0eStartedService\x127\n" +
	"\n" +
	"GetVersion\x12\x16.google.protobuf.Empty\x1a\x0f.daemon.Version\"\x00\x12K\n" +
	"\x16SubscribeServiceStatus\x12\x16.google.protobuf.Empty\x1a\x15.daemon.ServiceStatus\"\x000\x01\x127\n" +
	"\fSubscribeLog\x12\x16.google.protobuf.Empty\x1a\v.daemon.Log\"\x000\x01\x12G\n" +
	"\x12GetDefaultLogLevel\x12\x16.google.protobuf.Empty\x1a\x17.daemon.DefaultLogLevel\"\x00\x12=\n" +
	"\tClearLogs\x12\x16.google.protobuf.Empty\x1a\x16.google.protobuf.Empty\"\x00\x12E\n" +
	"\x0fSubscribeStatus\x12\x1e.daemon.SubscribeStatusRequest\x1a\x0e.daemon.Status\"\x000\x01\x12=\n" +
	"\x0fSubscribeGroups\x12\x16.google.protobuf.Empty\x1a\x0e.daemon.Groups\"\x000\x01\x12G\n" +
	"\x12GetClashModeStatus\x12\x16.google.protobuf.Empty\x1a\x17.daemon.ClashModeStatus\"\x00\x12C\n" +
	"\x12SubscribeClashMode\x12\x16.google.protobuf.Empty\x1a\x11.daemon.ClashMode\"\x000\x01\x12;\n" +
	"\fSetClashMode\x12\x11.daemon.ClashMode\x1a\x16.google.protobuf.Empty\"\x00\x12;\n" +
	"\aURLTest\x12\x16.daemon.URLTestRequest\x1a\x16.google.protobuf.Empty\"\x00\x12I\n" +
	"\x0eSelectOutbound\x12\x1d.daemon.SelectOutboundRequest\x1a\x16.google.protobuf.Empty\"\x00\x12I\n" +
	"\x0eSetGroupExpand\x12\x1d.daemon.SetGroupExpandRequest\x1a\x16.google.protobuf.Empty\"\x00\x12Y\n" +
	"\x14SubscribeConnections\x12#.daemon.SubscribeConnectionsRequest\x1a\x18.daemon.ConnectionEvents\"\x000\x01\x12K\n" +
	"\x0fCloseConnection\x12\x1e.daemon.CloseConnectionRequest\x1a\x16.google.protobuf.Empty\"\x00\x12G\n" +
	"\x13CloseAllConnections\x12\x16.google.protobuf.Empty\x1a\x16.google.protobuf.Empty\"\x00\x12M\n" +
	"\x15GetDeprecatedWarnings\x12\x16.google.protobuf.Empty\x1a\x1a.daemon.DeprecatedWarnings\"\x00\x12;\n" +
	"\fGetStartedAt\x12\x16.google.protobuf.Empty\x1a\x11.daemon.StartedAt\"\x00\x12F\n" +
	"\x12SubscribeOutbounds\x12\x16.google.protobuf.Empty\x1a\x14.daemon.OutboundList\"\x000\x01\x12d\n" +
	"\x17StartNetworkQualityTest\x12!.daemon.NetworkQualityTestRequest\x1a\".daemon.NetworkQualityTestProgress\"\x000\x01\x12F\n" +
	"\rStartSTUNTest\x12\x17.daemon.STUNTestRequest\x1a\x18.daemon.STUNTestProgress\"\x000\x01\x12U\n" +
	"\x18SubscribeTailscaleStatus\x12\x16.google.protobuf.Empty\x1a\x1d.daemon.TailscaleStatusUpdate\"\x000\x01\x12U\n" +
	"\x12StartTailscalePing\x12\x1c.daemon.TailscalePingRequest\x1a\x1d.daemon.TailscalePingResponse\"\x000\x01\x12U\n" +
	"\x14SetTailscaleExitNode\x12#.daemon.SetTailscaleExitNodeRequest\x1a\x16.google.protobuf.Empty\"\x00\x12K\n" +
	"\x0fTailscaleLogout\x12\x1e.daemon.TailscaleLogoutRequest\x1a\x16.google.protobuf.Empty\"\x00\x12f\n" +
	"\x18StartTailscaleSSHSession\x12!.daemon.TailscaleSSHClientMessage\x1a!.daemon.TailscaleSSHServerMessage\"\x00(\x010\x01\x12Z\n" +
	"\x16SubscribeTaildropInbox\x12%.daemon.SubscribeTaildropInboxRequest\x1a\x15.daemon.TaildropInbox\"\x000\x01\x12W\n" +
	"\x15MarkTaildropInboxRead\x12$.daemon.MarkTaildropInboxReadRequest\x1a\x16.google.protobuf.Empty\"\x00\x12_\n" +
	"\x11SendTaildropFiles\x12!.daemon.TaildropSendClientMessage\x1a!.daemon.TaildropSendServerMessage\"\x00(\x010\x01\x12b\n" +
	"\x14DownloadTaildropFile\x12#.daemon.DownloadTaildropFileRequest\x1a!.daemon.DownloadTaildropFileChunk\"\x000\x01\x12Q\n" +
	"\x12DeleteTaildropFile\x12!.daemon.DeleteTaildropFileRequest\x1a\x16.google.protobuf.Empty\"\x00\x12[\n" +
	"\x17CancelTaildropReceiving\x12&.daemon.CancelTaildropReceivingRequest\x1a\x16.google.protobuf.Empty\"\x00\x12O\n" +
	"\x11ProvideUSBDevices\x12\x1a.daemon.USBProviderMessage\x1a\x18.daemon.USBServerMessage\"\x00(\x010\x01\x12Y\n" +
	"\x1aSubscribeUSBIPServerStatus\x12\x16.google.protobuf.Empty\x1a\x1f.daemon.USBIPServerStatusUpdate\"\x000\x01\x12Y\n" +
	"\x1aSubscribeOpenConnectStatus\x12\x16.google.protobuf.Empty\x1a\x1f.daemon.OpenConnectStatusUpdate\"\x000\x01\x12d\n" +
	"\x1dSubmitOpenConnectAuthResponse\x12).daemon.OpenConnectAuthResponseSubmission\x1a\x16.google.protobuf.Empty\"\x00\x12b\n" +
	"\x1eCancelOpenConnectAuthChallenge\x12&.daemon.OpenConnectAuthChallengeCancel\x1a\x16.google.protobuf.Empty\"\x00\x12Q\n" +
	"\x16SubscribeOpenVPNStatus\x12\x16.google.protobuf.Empty\x1a\x1b.daemon.OpenVPNStatusUpdate\"\x000\x01\x12^\n" +
	"\x1eSubmitOpenVPNChallengeResponse\x12\".daemon.OpenVPNChallengeSubmission\x1a\x16.google.protobuf.Empty\"\x00\x12R\n" +
	"\x16CancelOpenVPNChallenge\x12\x1e.daemon.OpenVPNChallengeCancel\x1a\x16.google.protobuf.Empty\"\x00\x12O\n" +
	"\x16SubscribeNotifications\x12\x16.google.protobuf.Empty\x1a\x19.daemon.NotificationEvent\"\x000\x01\x12T\n" +
	"\x0fURLTestOutbound\x12\x1e.daemon.URLTestOutboundRequest\x1a\x1f.daemon.URLTestOutboundResponse\"\x00\x126\n" +
	"\bGetRules\x12\x16.google.protobuf.Empty\x1a\x10.daemon.RuleList\"\x00\x125\n" +
	"\tGetGroups\x12\x16.google.protobuf.Empty\x1a\x0e.daemon.Groups\"\x00\x12>\n" +
	"\fGetOutbounds\x12\x16.google.protobuf.Empty\x1a\x14.daemon.OutboundList\"\x00\x12T\n" +
	"\x13SubscribeDNSQueries\x12\".daemon.SubscribeDNSQueriesRequest\x1a\x15.daemon.DnsQueryEvent\"\x000\x01\x125\n" +
	"\aGetPool\x12\x16.daemon.GetPoolRequest\x1a\x10.daemon.PoolList\"\x00\x12>\n" +
	"\fGetDNSGroups\x12\x16.google.protobuf.Empty\x1a\x14.daemon.DnsGroupList\"\x00\x12C\n" +
	"\x10GetRunningConfig\x12\x16.google.protobuf.Empty\x1a\x15.daemon.RunningConfig\"\x00\x12Z\n" +
	"\x11GetURLViaOutbound\x12 .daemon.GetURLViaOutboundRequest\x1a!.daemon.GetURLViaOutboundResponse\"\x00\x128\n" +
	"\tGetChains\x12\x16.google.protobuf.Empty\x1a\x11.daemon.ChainList\"\x00\x12l\n" +
	"\x17SetChainPositionEnabled\x12&.daemon.SetChainPositionEnabledRequest\x1a'.daemon.SetChainPositionEnabledResponse\"\x00\x12R\n" +
	"\x13GetChainCloneConfig\x12\".daemon.GetChainCloneConfigRequest\x1a\x15.daemon.RunningConfig\"\x00B%Z#github.com/sagernet/sing-box/daemonb\x06proto3"

var (
	file_daemon_started_service_proto_rawDescOnce sync.Once
	file_daemon_started_service_proto_rawDescData []byte
)

func file_daemon_started_service_proto_rawDescGZIP() []byte {
	file_daemon_started_service_proto_rawDescOnce.Do(func() {
		file_daemon_started_service_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_daemon_started_service_proto_rawDesc), len(file_daemon_started_service_proto_rawDesc)))
	})
	return file_daemon_started_service_proto_rawDescData
}

var (
	file_daemon_started_service_proto_enumTypes = make([]protoimpl.EnumInfo, 5)
	file_daemon_started_service_proto_msgTypes  = make([]protoimpl.MessageInfo, 127)
	file_daemon_started_service_proto_goTypes   = []any{
		LogLevel(0),                               // 0: daemon.LogLevel
		ConnectionEventType(0),                    // 1: daemon.ConnectionEventType
		USBDeviceState(0),                         // 2: daemon.USBDeviceState
		USBBackend(0),                             // 3: daemon.USBBackend
		ServiceStatus_Type(0),                     // 4: daemon.ServiceStatus.Type
		(*Version)(nil),                           // 5: daemon.Version
		(*ServiceStatus)(nil),                     // 6: daemon.ServiceStatus
		(*SubscribeStatusRequest)(nil),            // 7: daemon.SubscribeStatusRequest
		(*Log)(nil),                               // 8: daemon.Log
		(*DefaultLogLevel)(nil),                   // 9: daemon.DefaultLogLevel
		(*Status)(nil),                            // 10: daemon.Status
		(*Groups)(nil),                            // 11: daemon.Groups
		(*Group)(nil),                             // 12: daemon.Group
		(*GroupItem)(nil),                         // 13: daemon.GroupItem
		(*URLTestRequest)(nil),                    // 14: daemon.URLTestRequest
		(*SelectOutboundRequest)(nil),             // 15: daemon.SelectOutboundRequest
		(*SetGroupExpandRequest)(nil),             // 16: daemon.SetGroupExpandRequest
		(*ClashMode)(nil),                         // 17: daemon.ClashMode
		(*ClashModeStatus)(nil),                   // 18: daemon.ClashModeStatus
		(*SubscribeConnectionsRequest)(nil),       // 19: daemon.SubscribeConnectionsRequest
		(*ConnectionEvent)(nil),                   // 20: daemon.ConnectionEvent
		(*ConnectionEvents)(nil),                  // 21: daemon.ConnectionEvents
		(*Connection)(nil),                        // 22: daemon.Connection
		(*ProcessInfo)(nil),                       // 23: daemon.ProcessInfo
		(*CloseConnectionRequest)(nil),            // 24: daemon.CloseConnectionRequest
		(*DeprecatedWarnings)(nil),                // 25: daemon.DeprecatedWarnings
		(*DeprecatedWarning)(nil),                 // 26: daemon.DeprecatedWarning
		(*StartedAt)(nil),                         // 27: daemon.StartedAt
		(*OutboundList)(nil),                      // 28: daemon.OutboundList
		(*NetworkQualityTestRequest)(nil),         // 29: daemon.NetworkQualityTestRequest
		(*NetworkQualityTestProgress)(nil),        // 30: daemon.NetworkQualityTestProgress
		(*STUNTestRequest)(nil),                   // 31: daemon.STUNTestRequest
		(*STUNTestProgress)(nil),                  // 32: daemon.STUNTestProgress
		(*TailscaleStatusUpdate)(nil),             // 33: daemon.TailscaleStatusUpdate
		(*TailscaleEndpointStatus)(nil),           // 34: daemon.TailscaleEndpointStatus
		(*TailscaleUserGroup)(nil),                // 35: daemon.TailscaleUserGroup
		(*TailscalePeer)(nil),                     // 36: daemon.TailscalePeer
		(*TailscalePingRequest)(nil),              // 37: daemon.TailscalePingRequest
		(*TailscalePingResponse)(nil),             // 38: daemon.TailscalePingResponse
		(*SetTailscaleExitNodeRequest)(nil),       // 39: daemon.SetTailscaleExitNodeRequest
		(*TailscaleLogoutRequest)(nil),            // 40: daemon.TailscaleLogoutRequest
		(*TailscaleSSHClientMessage)(nil),         // 41: daemon.TailscaleSSHClientMessage
		(*TailscaleSSHStart)(nil),                 // 42: daemon.TailscaleSSHStart
		(*TailscaleSSHInput)(nil),                 // 43: daemon.TailscaleSSHInput
		(*TailscaleSSHResize)(nil),                // 44: daemon.TailscaleSSHResize
		(*TailscaleSSHServerMessage)(nil),         // 45: daemon.TailscaleSSHServerMessage
		(*TailscaleSSHAuthBanner)(nil),            // 46: daemon.TailscaleSSHAuthBanner
		(*TailscaleSSHReady)(nil),                 // 47: daemon.TailscaleSSHReady
		(*TailscaleSSHOutput)(nil),                // 48: daemon.TailscaleSSHOutput
		(*TailscaleSSHExit)(nil),                  // 49: daemon.TailscaleSSHExit
		(*TailscaleSSHError)(nil),                 // 50: daemon.TailscaleSSHError
		(*SubscribeTaildropInboxRequest)(nil),     // 51: daemon.SubscribeTaildropInboxRequest
		(*MarkTaildropInboxReadRequest)(nil),      // 52: daemon.MarkTaildropInboxReadRequest
		(*TaildropInbox)(nil),                     // 53: daemon.TaildropInbox
		(*TaildropFile)(nil),                      // 54: daemon.TaildropFile
		(*TaildropReceivingFile)(nil),             // 55: daemon.TaildropReceivingFile
		(*TaildropSendClientMessage)(nil),         // 56: daemon.TaildropSendClientMessage
		(*TaildropSendStart)(nil),                 // 57: daemon.TaildropSendStart
		(*TaildropOutgoingFile)(nil),              // 58: daemon.TaildropOutgoingFile
		(*TaildropFileChunk)(nil),                 // 59: daemon.TaildropFileChunk
		(*TaildropFileDone)(nil),                  // 60: daemon.TaildropFileDone
		(*TaildropSendServerMessage)(nil),         // 61: daemon.TaildropSendServerMessage
		(*TaildropSendProgress)(nil),              // 62: daemon.TaildropSendProgress
		(*DownloadTaildropFileRequest)(nil),       // 63: daemon.DownloadTaildropFileRequest
		(*DownloadTaildropFileChunk)(nil),         // 64: daemon.DownloadTaildropFileChunk
		(*DeleteTaildropFileRequest)(nil),         // 65: daemon.DeleteTaildropFileRequest
		(*CancelTaildropReceivingRequest)(nil),    // 66: daemon.CancelTaildropReceivingRequest
		(*USBProviderMessage)(nil),                // 67: daemon.USBProviderMessage
		(*USBServerMessage)(nil),                  // 68: daemon.USBServerMessage
		(*USBDeviceDescriptor)(nil),               // 69: daemon.USBDeviceDescriptor
		(*USBDeviceAttach)(nil),                   // 70: daemon.USBDeviceAttach
		(*USBInterface)(nil),                      // 71: daemon.USBInterface
		(*USBDeviceDetach)(nil),                   // 72: daemon.USBDeviceDetach
		(*USBDeviceReady)(nil),                    // 73: daemon.USBDeviceReady
		(*USBURBRequest)(nil),                     // 74: daemon.USBURBRequest
		(*USBURBResponse)(nil),                    // 75: daemon.USBURBResponse
		(*USBIsoPacket)(nil),                      // 76: daemon.USBIsoPacket
		(*USBEndpointAbort)(nil),                  // 77: daemon.USBEndpointAbort
		(*USBError)(nil),                          // 78: daemon.USBError
		(*USBIPServerStatusUpdate)(nil),           // 79: daemon.USBIPServerStatusUpdate
		(*USBIPServerStatus)(nil),                 // 80: daemon.USBIPServerStatus
		(*USBSharedDevice)(nil),                   // 81: daemon.USBSharedDevice
		(*OpenConnectStatusUpdate)(nil),           // 82: daemon.OpenConnectStatusUpdate
		(*OpenConnectEndpointStatus)(nil),         // 83: daemon.OpenConnectEndpointStatus
		(*OpenConnectTunnelInfo)(nil),             // 84: daemon.OpenConnectTunnelInfo
		(*OpenConnectAuthChallenge)(nil),          // 85: daemon.OpenConnectAuthChallenge
		(*OpenConnectAuthForm)(nil),               // 86: daemon.OpenConnectAuthForm
		(*OpenConnectAuthFormField)(nil),          // 87: daemon.OpenConnectAuthFormField
		(*OpenConnectAuthFormChoice)(nil),         // 88: daemon.OpenConnectAuthFormChoice
		(*OpenConnectBrowserRequest)(nil),         // 89: daemon.OpenConnectBrowserRequest
		(*OpenConnectBrowserCookie)(nil),          // 90: daemon.OpenConnectBrowserCookie
		(*OpenConnectBrowserHeader)(nil),          // 91: daemon.OpenConnectBrowserHeader
		(*OpenConnectAuthFormResponse)(nil),       // 92: daemon.OpenConnectAuthFormResponse
		(*OpenConnectBrowserResult)(nil),          // 93: daemon.OpenConnectBrowserResult
		(*OpenConnectAuthResponseSubmission)(nil), // 94: daemon.OpenConnectAuthResponseSubmission
		(*OpenConnectAuthChallengeCancel)(nil),    // 95: daemon.OpenConnectAuthChallengeCancel
		(*OpenVPNStatusUpdate)(nil),               // 96: daemon.OpenVPNStatusUpdate
		(*OpenVPNEndpointStatus)(nil),             // 97: daemon.OpenVPNEndpointStatus
		(*OpenVPNTunnelInfo)(nil),                 // 98: daemon.OpenVPNTunnelInfo
		(*OpenVPNChallenge)(nil),                  // 99: daemon.OpenVPNChallenge
		(*OpenVPNChallengeSubmission)(nil),        // 100: daemon.OpenVPNChallengeSubmission
		(*OpenVPNChallengeCancel)(nil),            // 101: daemon.OpenVPNChallengeCancel
		(*NotificationEvent)(nil),                 // 102: daemon.NotificationEvent
		(*Notification)(nil),                      // 103: daemon.Notification
		(*NotificationCancel)(nil),                // 104: daemon.NotificationCancel
		(*URLTestOutboundRequest)(nil),            // 105: daemon.URLTestOutboundRequest
		(*URLTestOutboundResponse)(nil),           // 106: daemon.URLTestOutboundResponse
		(*HttpHeaderPair)(nil),                    // 107: daemon.HttpHeaderPair
		(*GetURLViaOutboundRequest)(nil),          // 108: daemon.GetURLViaOutboundRequest
		(*GetURLViaOutboundResponse)(nil),         // 109: daemon.GetURLViaOutboundResponse
		(*Rule)(nil),                              // 110: daemon.Rule
		(*RuleList)(nil),                          // 111: daemon.RuleList
		(*SubscribeDNSQueriesRequest)(nil),        // 112: daemon.SubscribeDNSQueriesRequest
		(*DnsQueryEvent)(nil),                     // 113: daemon.DnsQueryEvent
		(*DnsGroupAttempt)(nil),                   // 114: daemon.DnsGroupAttempt
		(*DnsAnswer)(nil),                         // 115: daemon.DnsAnswer
		(*GetPoolRequest)(nil),                    // 116: daemon.GetPoolRequest
		(*PoolSlot)(nil),                          // 117: daemon.PoolSlot
		(*PoolList)(nil),                          // 118: daemon.PoolList
		(*DnsGroupMember)(nil),                    // 119: daemon.DnsGroupMember
		(*DnsGroupState)(nil),                     // 120: daemon.DnsGroupState
		(*DnsGroupList)(nil),                      // 121: daemon.DnsGroupList
		(*RunningConfig)(nil),                     // 122: daemon.RunningConfig
		(*ChainCloneState)(nil),                   // 123: daemon.ChainCloneState
		(*ChainPosition)(nil),                     // 124: daemon.ChainPosition
		(*ChainState)(nil),                        // 125: daemon.ChainState
		(*ChainList)(nil),                         // 126: daemon.ChainList
		(*SetChainPositionEnabledRequest)(nil),    // 127: daemon.SetChainPositionEnabledRequest
		(*SetChainPositionEnabledResponse)(nil),   // 128: daemon.SetChainPositionEnabledResponse
		(*GetChainCloneConfigRequest)(nil),        // 129: daemon.GetChainCloneConfigRequest
		(*Log_Message)(nil),                       // 130: daemon.Log.Message
		nil,                                       // 131: daemon.OpenConnectAuthFormResponse.ValuesEntry
		(*emptypb.Empty)(nil),                     // 132: google.protobuf.Empty
	}
)

var file_daemon_started_service_proto_depIdxs = []int32{
	4,   // 0: daemon.ServiceStatus.status:type_name -> daemon.ServiceStatus.Type
	130, // 1: daemon.Log.messages:type_name -> daemon.Log.Message
	0,   // 2: daemon.DefaultLogLevel.level:type_name -> daemon.LogLevel
	12,  // 3: daemon.Groups.group:type_name -> daemon.Group
	13,  // 4: daemon.Group.items:type_name -> daemon.GroupItem
	1,   // 5: daemon.ConnectionEvent.type:type_name -> daemon.ConnectionEventType
	22,  // 6: daemon.ConnectionEvent.connection:type_name -> daemon.Connection
	20,  // 7: daemon.ConnectionEvents.events:type_name -> daemon.ConnectionEvent
	23,  // 8: daemon.Connection.processInfo:type_name -> daemon.ProcessInfo
	26,  // 9: daemon.DeprecatedWarnings.warnings:type_name -> daemon.DeprecatedWarning
	13,  // 10: daemon.OutboundList.outbounds:type_name -> daemon.GroupItem
	34,  // 11: daemon.TailscaleStatusUpdate.endpoints:type_name -> daemon.TailscaleEndpointStatus
	36,  // 12: daemon.TailscaleEndpointStatus.self:type_name -> daemon.TailscalePeer
	35,  // 13: daemon.TailscaleEndpointStatus.userGroups:type_name -> daemon.TailscaleUserGroup
	36,  // 14: daemon.TailscaleEndpointStatus.exitNode:type_name -> daemon.TailscalePeer
	36,  // 15: daemon.TailscaleUserGroup.peers:type_name -> daemon.TailscalePeer
	42,  // 16: daemon.TailscaleSSHClientMessage.start:type_name -> daemon.TailscaleSSHStart
	43,  // 17: daemon.TailscaleSSHClientMessage.input:type_name -> daemon.TailscaleSSHInput
	44,  // 18: daemon.TailscaleSSHClientMessage.resize:type_name -> daemon.TailscaleSSHResize
	46,  // 19: daemon.TailscaleSSHServerMessage.authBanner:type_name -> daemon.TailscaleSSHAuthBanner
	47,  // 20: daemon.TailscaleSSHServerMessage.ready:type_name -> daemon.TailscaleSSHReady
	48,  // 21: daemon.TailscaleSSHServerMessage.output:type_name -> daemon.TailscaleSSHOutput
	49,  // 22: daemon.TailscaleSSHServerMessage.exit:type_name -> daemon.TailscaleSSHExit
	50,  // 23: daemon.TailscaleSSHServerMessage.error:type_name -> daemon.TailscaleSSHError
	54,  // 24: daemon.TaildropInbox.files:type_name -> daemon.TaildropFile
	55,  // 25: daemon.TaildropInbox.receiving:type_name -> daemon.TaildropReceivingFile
	57,  // 26: daemon.TaildropSendClientMessage.start:type_name -> daemon.TaildropSendStart
	59,  // 27: daemon.TaildropSendClientMessage.chunk:type_name -> daemon.TaildropFileChunk
	60,  // 28: daemon.TaildropSendClientMessage.fileDone:type_name -> daemon.TaildropFileDone
	58,  // 29: daemon.TaildropSendStart.files:type_name -> daemon.TaildropOutgoingFile
	62,  // 30: daemon.TaildropSendServerMessage.progress:type_name -> daemon.TaildropSendProgress
	70,  // 31: daemon.USBProviderMessage.attach:type_name -> daemon.USBDeviceAttach
	72,  // 32: daemon.USBProviderMessage.detach:type_name -> daemon.USBDeviceDetach
	75,  // 33: daemon.USBProviderMessage.urbResponse:type_name -> daemon.USBURBResponse
	73,  // 34: daemon.USBServerMessage.ready:type_name -> daemon.USBDeviceReady
	74,  // 35: daemon.USBServerMessage.urbRequest:type_name -> daemon.USBURBRequest
	77,  // 36: daemon.USBServerMessage.abort:type_name -> daemon.USBEndpointAbort
	78,  // 37: daemon.USBServerMessage.error:type_name -> daemon.USBError
	71,  // 38: daemon.USBDeviceDescriptor.interfaces:type_name -> daemon.USBInterface
	69,  // 39: daemon.USBDeviceAttach.descriptor:type_name -> daemon.USBDeviceDescriptor
	76,  // 40: daemon.USBURBRequest.isoPackets:type_name -> daemon.USBIsoPacket
	76,  // 41: daemon.USBURBResponse.isoPackets:type_name -> daemon.USBIsoPacket
	80,  // 42: daemon.USBIPServerStatusUpdate.servers:type_name -> daemon.USBIPServerStatus
	81,  // 43: daemon.USBIPServerStatus.devices:type_name -> daemon.USBSharedDevice
	69,  // 44: daemon.USBSharedDevice.descriptor:type_name -> daemon.USBDeviceDescriptor
	3,   // 45: daemon.USBSharedDevice.backend:type_name -> daemon.USBBackend
	2,   // 46: daemon.USBSharedDevice.state:type_name -> daemon.USBDeviceState
	83,  // 47: daemon.OpenConnectStatusUpdate.endpoints:type_name -> daemon.OpenConnectEndpointStatus
	85,  // 48: daemon.OpenConnectEndpointStatus.authChallenge:type_name -> daemon.OpenConnectAuthChallenge
	84,  // 49: daemon.OpenConnectEndpointStatus.tunnelInfo:type_name -> daemon.OpenConnectTunnelInfo
	86,  // 50: daemon.OpenConnectAuthChallenge.form:type_name -> daemon.OpenConnectAuthForm
	89,  // 51: daemon.OpenConnectAuthChallenge.browser:type_name -> daemon.OpenConnectBrowserRequest
	87,  // 52: daemon.OpenConnectAuthForm.fields:type_name -> daemon.OpenConnectAuthFormField
	88,  // 53: daemon.OpenConnectAuthFormField.options:type_name -> daemon.OpenConnectAuthFormChoice
	131, // 54: daemon.OpenConnectAuthFormResponse.values:type_name -> daemon.OpenConnectAuthFormResponse.ValuesEntry
	90,  // 55: daemon.OpenConnectBrowserResult.cookies:type_name -> daemon.OpenConnectBrowserCookie
	91,  // 56: daemon.OpenConnectBrowserResult.headers:type_name -> daemon.OpenConnectBrowserHeader
	92,  // 57: daemon.OpenConnectAuthResponseSubmission.form:type_name -> daemon.OpenConnectAuthFormResponse
	93,  // 58: daemon.OpenConnectAuthResponseSubmission.browser:type_name -> daemon.OpenConnectBrowserResult
	97,  // 59: daemon.OpenVPNStatusUpdate.endpoints:type_name -> daemon.OpenVPNEndpointStatus
	99,  // 60: daemon.OpenVPNEndpointStatus.challenge:type_name -> daemon.OpenVPNChallenge
	98,  // 61: daemon.OpenVPNEndpointStatus.tunnelInfo:type_name -> daemon.OpenVPNTunnelInfo
	103, // 62: daemon.NotificationEvent.send:type_name -> daemon.Notification
	104, // 63: daemon.NotificationEvent.cancel:type_name -> daemon.NotificationCancel
	107, // 64: daemon.GetURLViaOutboundRequest.headers:type_name -> daemon.HttpHeaderPair
	110, // 65: daemon.RuleList.rules:type_name -> daemon.Rule
	23,  // 66: daemon.DnsQueryEvent.processInfo:type_name -> daemon.ProcessInfo
	115, // 67: daemon.DnsQueryEvent.answers:type_name -> daemon.DnsAnswer
	114, // 68: daemon.DnsQueryEvent.attempts:type_name -> daemon.DnsGroupAttempt
	117, // 69: daemon.PoolList.slots:type_name -> daemon.PoolSlot
	119, // 70: daemon.DnsGroupState.members:type_name -> daemon.DnsGroupMember
	120, // 71: daemon.DnsGroupList.groups:type_name -> daemon.DnsGroupState
	123, // 72: daemon.ChainPosition.clone:type_name -> daemon.ChainCloneState
	124, // 73: daemon.ChainState.positions:type_name -> daemon.ChainPosition
	125, // 74: daemon.ChainList.chains:type_name -> daemon.ChainState
	0,   // 75: daemon.Log.Message.level:type_name -> daemon.LogLevel
	132, // 76: daemon.StartedService.GetVersion:input_type -> google.protobuf.Empty
	132, // 77: daemon.StartedService.SubscribeServiceStatus:input_type -> google.protobuf.Empty
	132, // 78: daemon.StartedService.SubscribeLog:input_type -> google.protobuf.Empty
	132, // 79: daemon.StartedService.GetDefaultLogLevel:input_type -> google.protobuf.Empty
	132, // 80: daemon.StartedService.ClearLogs:input_type -> google.protobuf.Empty
	7,   // 81: daemon.StartedService.SubscribeStatus:input_type -> daemon.SubscribeStatusRequest
	132, // 82: daemon.StartedService.SubscribeGroups:input_type -> google.protobuf.Empty
	132, // 83: daemon.StartedService.GetClashModeStatus:input_type -> google.protobuf.Empty
	132, // 84: daemon.StartedService.SubscribeClashMode:input_type -> google.protobuf.Empty
	17,  // 85: daemon.StartedService.SetClashMode:input_type -> daemon.ClashMode
	14,  // 86: daemon.StartedService.URLTest:input_type -> daemon.URLTestRequest
	15,  // 87: daemon.StartedService.SelectOutbound:input_type -> daemon.SelectOutboundRequest
	16,  // 88: daemon.StartedService.SetGroupExpand:input_type -> daemon.SetGroupExpandRequest
	19,  // 89: daemon.StartedService.SubscribeConnections:input_type -> daemon.SubscribeConnectionsRequest
	24,  // 90: daemon.StartedService.CloseConnection:input_type -> daemon.CloseConnectionRequest
	132, // 91: daemon.StartedService.CloseAllConnections:input_type -> google.protobuf.Empty
	132, // 92: daemon.StartedService.GetDeprecatedWarnings:input_type -> google.protobuf.Empty
	132, // 93: daemon.StartedService.GetStartedAt:input_type -> google.protobuf.Empty
	132, // 94: daemon.StartedService.SubscribeOutbounds:input_type -> google.protobuf.Empty
	29,  // 95: daemon.StartedService.StartNetworkQualityTest:input_type -> daemon.NetworkQualityTestRequest
	31,  // 96: daemon.StartedService.StartSTUNTest:input_type -> daemon.STUNTestRequest
	132, // 97: daemon.StartedService.SubscribeTailscaleStatus:input_type -> google.protobuf.Empty
	37,  // 98: daemon.StartedService.StartTailscalePing:input_type -> daemon.TailscalePingRequest
	39,  // 99: daemon.StartedService.SetTailscaleExitNode:input_type -> daemon.SetTailscaleExitNodeRequest
	40,  // 100: daemon.StartedService.TailscaleLogout:input_type -> daemon.TailscaleLogoutRequest
	41,  // 101: daemon.StartedService.StartTailscaleSSHSession:input_type -> daemon.TailscaleSSHClientMessage
	51,  // 102: daemon.StartedService.SubscribeTaildropInbox:input_type -> daemon.SubscribeTaildropInboxRequest
	52,  // 103: daemon.StartedService.MarkTaildropInboxRead:input_type -> daemon.MarkTaildropInboxReadRequest
	56,  // 104: daemon.StartedService.SendTaildropFiles:input_type -> daemon.TaildropSendClientMessage
	63,  // 105: daemon.StartedService.DownloadTaildropFile:input_type -> daemon.DownloadTaildropFileRequest
	65,  // 106: daemon.StartedService.DeleteTaildropFile:input_type -> daemon.DeleteTaildropFileRequest
	66,  // 107: daemon.StartedService.CancelTaildropReceiving:input_type -> daemon.CancelTaildropReceivingRequest
	67,  // 108: daemon.StartedService.ProvideUSBDevices:input_type -> daemon.USBProviderMessage
	132, // 109: daemon.StartedService.SubscribeUSBIPServerStatus:input_type -> google.protobuf.Empty
	132, // 110: daemon.StartedService.SubscribeOpenConnectStatus:input_type -> google.protobuf.Empty
	94,  // 111: daemon.StartedService.SubmitOpenConnectAuthResponse:input_type -> daemon.OpenConnectAuthResponseSubmission
	95,  // 112: daemon.StartedService.CancelOpenConnectAuthChallenge:input_type -> daemon.OpenConnectAuthChallengeCancel
	132, // 113: daemon.StartedService.SubscribeOpenVPNStatus:input_type -> google.protobuf.Empty
	100, // 114: daemon.StartedService.SubmitOpenVPNChallengeResponse:input_type -> daemon.OpenVPNChallengeSubmission
	101, // 115: daemon.StartedService.CancelOpenVPNChallenge:input_type -> daemon.OpenVPNChallengeCancel
	132, // 116: daemon.StartedService.SubscribeNotifications:input_type -> google.protobuf.Empty
	105, // 117: daemon.StartedService.URLTestOutbound:input_type -> daemon.URLTestOutboundRequest
	132, // 118: daemon.StartedService.GetRules:input_type -> google.protobuf.Empty
	132, // 119: daemon.StartedService.GetGroups:input_type -> google.protobuf.Empty
	132, // 120: daemon.StartedService.GetOutbounds:input_type -> google.protobuf.Empty
	112, // 121: daemon.StartedService.SubscribeDNSQueries:input_type -> daemon.SubscribeDNSQueriesRequest
	116, // 122: daemon.StartedService.GetPool:input_type -> daemon.GetPoolRequest
	132, // 123: daemon.StartedService.GetDNSGroups:input_type -> google.protobuf.Empty
	132, // 124: daemon.StartedService.GetRunningConfig:input_type -> google.protobuf.Empty
	108, // 125: daemon.StartedService.GetURLViaOutbound:input_type -> daemon.GetURLViaOutboundRequest
	132, // 126: daemon.StartedService.GetChains:input_type -> google.protobuf.Empty
	127, // 127: daemon.StartedService.SetChainPositionEnabled:input_type -> daemon.SetChainPositionEnabledRequest
	129, // 128: daemon.StartedService.GetChainCloneConfig:input_type -> daemon.GetChainCloneConfigRequest
	5,   // 129: daemon.StartedService.GetVersion:output_type -> daemon.Version
	6,   // 130: daemon.StartedService.SubscribeServiceStatus:output_type -> daemon.ServiceStatus
	8,   // 131: daemon.StartedService.SubscribeLog:output_type -> daemon.Log
	9,   // 132: daemon.StartedService.GetDefaultLogLevel:output_type -> daemon.DefaultLogLevel
	132, // 133: daemon.StartedService.ClearLogs:output_type -> google.protobuf.Empty
	10,  // 134: daemon.StartedService.SubscribeStatus:output_type -> daemon.Status
	11,  // 135: daemon.StartedService.SubscribeGroups:output_type -> daemon.Groups
	18,  // 136: daemon.StartedService.GetClashModeStatus:output_type -> daemon.ClashModeStatus
	17,  // 137: daemon.StartedService.SubscribeClashMode:output_type -> daemon.ClashMode
	132, // 138: daemon.StartedService.SetClashMode:output_type -> google.protobuf.Empty
	132, // 139: daemon.StartedService.URLTest:output_type -> google.protobuf.Empty
	132, // 140: daemon.StartedService.SelectOutbound:output_type -> google.protobuf.Empty
	132, // 141: daemon.StartedService.SetGroupExpand:output_type -> google.protobuf.Empty
	21,  // 142: daemon.StartedService.SubscribeConnections:output_type -> daemon.ConnectionEvents
	132, // 143: daemon.StartedService.CloseConnection:output_type -> google.protobuf.Empty
	132, // 144: daemon.StartedService.CloseAllConnections:output_type -> google.protobuf.Empty
	25,  // 145: daemon.StartedService.GetDeprecatedWarnings:output_type -> daemon.DeprecatedWarnings
	27,  // 146: daemon.StartedService.GetStartedAt:output_type -> daemon.StartedAt
	28,  // 147: daemon.StartedService.SubscribeOutbounds:output_type -> daemon.OutboundList
	30,  // 148: daemon.StartedService.StartNetworkQualityTest:output_type -> daemon.NetworkQualityTestProgress
	32,  // 149: daemon.StartedService.StartSTUNTest:output_type -> daemon.STUNTestProgress
	33,  // 150: daemon.StartedService.SubscribeTailscaleStatus:output_type -> daemon.TailscaleStatusUpdate
	38,  // 151: daemon.StartedService.StartTailscalePing:output_type -> daemon.TailscalePingResponse
	132, // 152: daemon.StartedService.SetTailscaleExitNode:output_type -> google.protobuf.Empty
	132, // 153: daemon.StartedService.TailscaleLogout:output_type -> google.protobuf.Empty
	45,  // 154: daemon.StartedService.StartTailscaleSSHSession:output_type -> daemon.TailscaleSSHServerMessage
	53,  // 155: daemon.StartedService.SubscribeTaildropInbox:output_type -> daemon.TaildropInbox
	132, // 156: daemon.StartedService.MarkTaildropInboxRead:output_type -> google.protobuf.Empty
	61,  // 157: daemon.StartedService.SendTaildropFiles:output_type -> daemon.TaildropSendServerMessage
	64,  // 158: daemon.StartedService.DownloadTaildropFile:output_type -> daemon.DownloadTaildropFileChunk
	132, // 159: daemon.StartedService.DeleteTaildropFile:output_type -> google.protobuf.Empty
	132, // 160: daemon.StartedService.CancelTaildropReceiving:output_type -> google.protobuf.Empty
	68,  // 161: daemon.StartedService.ProvideUSBDevices:output_type -> daemon.USBServerMessage
	79,  // 162: daemon.StartedService.SubscribeUSBIPServerStatus:output_type -> daemon.USBIPServerStatusUpdate
	82,  // 163: daemon.StartedService.SubscribeOpenConnectStatus:output_type -> daemon.OpenConnectStatusUpdate
	132, // 164: daemon.StartedService.SubmitOpenConnectAuthResponse:output_type -> google.protobuf.Empty
	132, // 165: daemon.StartedService.CancelOpenConnectAuthChallenge:output_type -> google.protobuf.Empty
	96,  // 166: daemon.StartedService.SubscribeOpenVPNStatus:output_type -> daemon.OpenVPNStatusUpdate
	132, // 167: daemon.StartedService.SubmitOpenVPNChallengeResponse:output_type -> google.protobuf.Empty
	132, // 168: daemon.StartedService.CancelOpenVPNChallenge:output_type -> google.protobuf.Empty
	102, // 169: daemon.StartedService.SubscribeNotifications:output_type -> daemon.NotificationEvent
	106, // 170: daemon.StartedService.URLTestOutbound:output_type -> daemon.URLTestOutboundResponse
	111, // 171: daemon.StartedService.GetRules:output_type -> daemon.RuleList
	11,  // 172: daemon.StartedService.GetGroups:output_type -> daemon.Groups
	28,  // 173: daemon.StartedService.GetOutbounds:output_type -> daemon.OutboundList
	113, // 174: daemon.StartedService.SubscribeDNSQueries:output_type -> daemon.DnsQueryEvent
	118, // 175: daemon.StartedService.GetPool:output_type -> daemon.PoolList
	121, // 176: daemon.StartedService.GetDNSGroups:output_type -> daemon.DnsGroupList
	122, // 177: daemon.StartedService.GetRunningConfig:output_type -> daemon.RunningConfig
	109, // 178: daemon.StartedService.GetURLViaOutbound:output_type -> daemon.GetURLViaOutboundResponse
	126, // 179: daemon.StartedService.GetChains:output_type -> daemon.ChainList
	128, // 180: daemon.StartedService.SetChainPositionEnabled:output_type -> daemon.SetChainPositionEnabledResponse
	122, // 181: daemon.StartedService.GetChainCloneConfig:output_type -> daemon.RunningConfig
	129, // [129:182] is the sub-list for method output_type
	76,  // [76:129] is the sub-list for method input_type
	76,  // [76:76] is the sub-list for extension type_name
	76,  // [76:76] is the sub-list for extension extendee
	0,   // [0:76] is the sub-list for field type_name
}

func init() { file_daemon_started_service_proto_init() }
func file_daemon_started_service_proto_init() {
	if File_daemon_started_service_proto != nil {
		return
	}
	file_daemon_started_service_proto_msgTypes[36].OneofWrappers = []any{
		(*TailscaleSSHClientMessage_Start)(nil),
		(*TailscaleSSHClientMessage_Input)(nil),
		(*TailscaleSSHClientMessage_Resize)(nil),
	}
	file_daemon_started_service_proto_msgTypes[40].OneofWrappers = []any{
		(*TailscaleSSHServerMessage_AuthBanner)(nil),
		(*TailscaleSSHServerMessage_Ready)(nil),
		(*TailscaleSSHServerMessage_Output)(nil),
		(*TailscaleSSHServerMessage_Exit)(nil),
		(*TailscaleSSHServerMessage_Error)(nil),
	}
	file_daemon_started_service_proto_msgTypes[51].OneofWrappers = []any{
		(*TaildropSendClientMessage_Start)(nil),
		(*TaildropSendClientMessage_Chunk)(nil),
		(*TaildropSendClientMessage_FileDone)(nil),
	}
	file_daemon_started_service_proto_msgTypes[56].OneofWrappers = []any{
		(*TaildropSendServerMessage_Progress)(nil),
		(*TaildropSendServerMessage_ReceivedBytes)(nil),
	}
	file_daemon_started_service_proto_msgTypes[62].OneofWrappers = []any{
		(*USBProviderMessage_Attach)(nil),
		(*USBProviderMessage_Detach)(nil),
		(*USBProviderMessage_UrbResponse)(nil),
	}
	file_daemon_started_service_proto_msgTypes[63].OneofWrappers = []any{
		(*USBServerMessage_Ready)(nil),
		(*USBServerMessage_UrbRequest)(nil),
		(*USBServerMessage_Abort)(nil),
		(*USBServerMessage_Error)(nil),
	}
	file_daemon_started_service_proto_msgTypes[80].OneofWrappers = []any{
		(*OpenConnectAuthChallenge_Form)(nil),
		(*OpenConnectAuthChallenge_Browser)(nil),
	}
	file_daemon_started_service_proto_msgTypes[89].OneofWrappers = []any{
		(*OpenConnectAuthResponseSubmission_Form)(nil),
		(*OpenConnectAuthResponseSubmission_Browser)(nil),
	}
	file_daemon_started_service_proto_msgTypes[97].OneofWrappers = []any{
		(*NotificationEvent_Send)(nil),
		(*NotificationEvent_Cancel)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_daemon_started_service_proto_rawDesc), len(file_daemon_started_service_proto_rawDesc)),
			NumEnums:      5,
			NumMessages:   127,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_daemon_started_service_proto_goTypes,
		DependencyIndexes: file_daemon_started_service_proto_depIdxs,
		EnumInfos:         file_daemon_started_service_proto_enumTypes,
		MessageInfos:      file_daemon_started_service_proto_msgTypes,
	}.Build()
	File_daemon_started_service_proto = out.File
	file_daemon_started_service_proto_goTypes = nil
	file_daemon_started_service_proto_depIdxs = nil
}
