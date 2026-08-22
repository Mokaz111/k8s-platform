package errcode

import (
	"fmt"
	"net/http"
)

// 统一错误码结构。参考 gRPC 错误码分类 + HTTP 语义映射
type Code int

const (
	// 成功
	OK Code = 0

	// 客户端错误 4xxx 映射
	InvalidArgument  Code = 40001 // 请求参数错误
	Unauthenticated  Code = 40101 // 未认证
	TokenExpired     Code = 40102 // Token 过期
	PermissionDenied Code = 40301 // 无该功能权限
	ScopeDenied      Code = 40302 // 数据范围越权
	NotFound         Code = 40401 // 资源不存在
	AlreadyExists    Code = 40901 // 资源已存在

	// 业务错误 5xxx 映射（保留 1xx 业务分类空间）
	ClusterConnectFail   Code = 51001 // 集群连接失败
	ClusterOffline       Code = 51002 // 集群离线
	KubeconfigInvalid    Code = 51003 // kubeconfig 格式无效
	KubeconfigExpire     Code = 51004 // 证书/Token 过期
	K8SAPIError          Code = 51101 // K8S API 返回错误
	K8SAPIRateLimited    Code = 51102 // 触发 API Server 限流/熔断
	K8SResourceConflict  Code = 51103 // ResourceVersion 冲突
	VersionNotFound      Code = 52001 // 指定版本快照不存在
	VersionRollbackFail  Code = 52002 // 版本回滚失败
	BackupNotFound       Code = 53001 // 备份不存在
	BackupRestoreFail    Code = 53002 // 备份恢复失败
	BackupStorageError   Code = 53003 // 备份存储后端错误
	PluginNotLoaded      Code = 54001 // 插件未加载或禁用
	PluginInvokeFail     Code = 54002 // 插件调用失败

	// 系统错误
	Internal       Code = 50001 // 系统内部错误
	DatabaseError  Code = 50002 // 数据库异常
	RedisError     Code = 50003 // Redis 异常
	KMSDecryptFail Code = 50004 // 凭证解密失败
)

// 通用文字映射
var codeMsg = map[Code]string{
	OK:                   "OK",
	InvalidArgument:      "请求参数错误",
	Unauthenticated:      "未认证或登录已失效",
	TokenExpired:         "Token 已过期",
	PermissionDenied:     "无权限执行该操作",
	ScopeDenied:          "无权访问该集群/命名空间",
	NotFound:             "请求资源不存在",
	AlreadyExists:        "资源已存在",
	ClusterConnectFail:   "集群连接失败，请检查网络或 kubeconfig",
	ClusterOffline:       "集群当前离线，请稍后重试",
	KubeconfigInvalid:    "kubeconfig 格式无效或缺失必要字段",
	KubeconfigExpire:     "kubeconfig 凭证已过期，请重新导入",
	K8SAPIError:          "Kubernetes API 请求失败",
	K8SAPIRateLimited:    "集群 API Server 繁忙，请求已限流，请稍后重试",
	K8SResourceConflict:  "资源已被他人修改，请刷新后重试",
	VersionNotFound:      "指定的版本历史不存在",
	VersionRollbackFail:  "版本回滚失败",
	BackupNotFound:       "备份记录不存在",
	BackupRestoreFail:    "从备份恢复失败",
	BackupStorageError:   "备份存储访问失败",
	PluginNotLoaded:      "插件未启用或加载失败",
	PluginInvokeFail:     "插件执行失败",
	Internal:             "服务器内部错误",
	DatabaseError:        "数据库访问异常",
	RedisError:           "缓存访问异常",
	KMSDecryptFail:       "凭证解密失败，请检查密钥配置",
}

// HTTPStatus 返回错误码对应的 HTTP 状态码
func (c Code) HTTPStatus() int {
	switch {
	case c == OK:
		return http.StatusOK
	case c >= 40000 && c < 40100:
		return http.StatusBadRequest
	case c >= 40100 && c < 40300:
		return http.StatusUnauthorized
	case c >= 40300 && c < 40400:
		return http.StatusForbidden
	case c >= 40400 && c < 40900:
		return http.StatusNotFound
	case c >= 40900 && c < 50000:
		return http.StatusConflict
	case c >= 50000:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func (c Code) Msg() string {
	if m, ok := codeMsg[c]; ok {
		return m
	}
	return "Unknown Error"
}

// Error 统一错误结构，用于 Gin Handler 返回
type Error struct {
	Code    Code        `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// New 创建错误
func New(code Code, msgAndArgs ...interface{}) *Error {
	e := &Error{Code: code, Message: code.Msg()}
	if len(msgAndArgs) > 0 {
		if len(msgAndArgs) == 1 {
			if s, ok := msgAndArgs[0].(string); ok {
				e.Message = s
			} else {
				e.Details = msgAndArgs[0]
			}
		} else if len(msgAndArgs) == 2 {
			e.Message, _ = msgAndArgs[0].(string)
			e.Details = msgAndArgs[1]
		} else {
			e.Message = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
	}
	return e
}

// Wrap 包装底层错误（Details 里带原始错误信息）
func Wrap(code Code, err error, msgAndArgs ...interface{}) *Error {
	e := New(code, msgAndArgs...)
	if err != nil {
		if e.Details == nil {
			e.Details = err.Error()
		} else {
			e.Details = map[string]interface{}{"cause": err.Error(), "info": e.Details}
		}
	}
	return e
}
