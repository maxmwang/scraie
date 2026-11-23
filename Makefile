gen:
	protoc --go_out=. --go_opt=module=github.com/maxmwang/scraie \
		--go-grpc_out=. --go-grpc_opt=module=github.com/maxmwang/scraie \
		internal/search/pb/request.proto
