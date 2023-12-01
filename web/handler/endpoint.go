package handler

import (
	"context"

	"github.com/go-kit/kit/endpoint"
)

type SvcManagerRequest struct {
}

type SvcManagerResponse struct {
	V   string `json:"v,omitempty"`
	Err string `json:"err,omitempty"`
}

// Restart
//
//	@Summary 		重启daemon进程和所有子进程
//	@Description	?update 可以选择是否更新配置文件daemon.yml
//	@Tags			Restart
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	SvcManagerResponse
//	@Router			/restart [put]
func MakeRestartEndpoint(svcManager SvcManager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		err := svcManager.Restart()
		if err != nil {
			return SvcManagerResponse{Err: err.Error()}, nil
		}
		return SvcManagerResponse{V: "ok"}, nil
	}
}

// Reload
//
//	@Summary 				reload守护进程和子进程
//	@Description	?update 可以选择是否更新配置文件daemon.yml
//	@Tags			Reload
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	SvcManagerResponse
//	@Router			/reload [put]
func MakeReloadEndpoint(svcManager SvcManager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		err := svcManager.Reload()
		if err != nil {
			return SvcManagerResponse{Err: err.Error()}, nil
		}
		return SvcManagerResponse{V: "ok"}, nil
	}
}

// List
//
//	@Summary 				列出所有子进程的端口和命令
//	@Tags			Reload
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	SvcManagerResponse
//	@Router			/list [get]
func MakeListEndpoint(svcManager SvcManager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		return SvcManagerResponse{V: string(svcManager.List())}, nil
	}
}

// Update
//
//	@Summary 				更新配置文件
//	@Tags			Update
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	SvcManagerResponse
//	@Router			/update [put]
func MakeUpdateEndpoint(svcManager SvcManager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		err := svcManager.Update()
		if err != nil {
			return SvcManagerResponse{Err: err.Error()}, nil
		}
		return SvcManagerResponse{V: "ok"}, nil
	}
}

// Stop
//
//	@Summary 				更新配置文件
//	@Tags			Stop
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	SvcManagerResponse
//	@Router			/stop [put]
func MakeStopEndpoint(svcManager SvcManager) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		err := svcManager.Stop()
		if err != nil {
			return SvcManagerResponse{Err: err.Error()}, nil
		}
		return SvcManagerResponse{V: "ok"}, nil
	}
}
