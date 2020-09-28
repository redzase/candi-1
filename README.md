# Candi, a framework for `Membangun Candi` project in Golang

## Made with
<p align="center">
  <img src="https://storage.googleapis.com/agungdp/static/logo/golang.png" width="80" alt="golang logo" />
  <img src="https://storage.googleapis.com/agungdp/static/logo/docker.png" width="80" hspace="10" alt="docker logo" />
  <img src="https://storage.googleapis.com/agungdp/static/logo/rest.png" width="80" hspace="10" alt="rest logo" />
  <img src="https://storage.googleapis.com/agungdp/static/logo/graphql.png" width="80" alt="graphql logo" />
  <img src="https://storage.googleapis.com/agungdp/static/logo/grpc.png" width="160" hspace="15" vspace="15" alt="grpc logo" />
  <img src="https://storage.googleapis.com/agungdp/static/logo/kafka.png" height="80" alt="kafka logo" />
</p>


### Install `binary` from source
```
$ go get pkg.agungdwiprasetyo.com/candi
$ go install pkg.agungdwiprasetyo.com/candi/cmd/candi
```


### Create new service
```
$ candi --scope=initservice --servicename={{service_name}} --modules={{module_a}},{{module_b}}
```


### Add new modules in existing service
```
$ candi --scope=addmodule --servicename={{service_name}} --modules={{module_c}},{{module_d}}
```


## Service handlers example:
* [**Example Cron worker in delivery layer**](https://github.com/agungdwiprasetyo/backend-microservices/tree/master/pkg/codebase/app/cron_worker)
* [**Example Kafka consumer in delivery layer**](https://github.com/agungdwiprasetyo/backend-microservices/tree/master/pkg/codebase/app/kafka_worker)
* [**Example Redis subscriber in delivery layer**](https://github.com/agungdwiprasetyo/backend-microservices/tree/master/pkg/codebase/app/redis_worker)
* [**Example Task queue worker in delivery layer**](https://github.com/agungdwiprasetyo/backend-microservices/tree/master/pkg/codebase/app/task_queue_worker)


## Todo
- [x] ~~Add task queue worker like celery~~ Add UI for manage task queue worker
