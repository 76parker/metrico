# go-musthave-metrics-tpl

Шаблон репозитория для трека «Сервер сбора метрик и алертинга».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Бенчмарки и профилирование памяти

Проблема была в создании gzip-компрессора в middleware для каждого исходящего http-запроса (internal/api/middleware/compress.go. Исправил это с помощью `sync.Pool` который менеджит переиспользуемые gzip-компрессоры. 

- `BenchmarkMetricsBatch` — подготовка фиксированного набора runtime-метрик агента;
- `BenchmarkMemStorage` — пакетная запись, чтение метрики и снимок хранилища;
- `BenchmarkMakeBatch` — подготовка PostgreSQL-батча без выполнения SQL;
- `BenchmarkBatchUpdateFromJSON` — HTTP-обработчик с проверяющей заглушкой приложения;
- `BenchmarkCompress` — gzip-ответ через настоящий Gin middleware.

### Результаты профилирования

| Показатель pprof | До | После | Изменение |
| --- | ---: | ---: | ---: |
| `alloc_space`, MiB | 6554,02 | 4686,93 | −1867,09 (−28,49%) |
| `alloc_objects`, объектов | 39 981 899 | 42 502 276 | +2 520 377 (+6,30%) |
| `inuse_space` после GC, KiB | 4395,84 | 5669,49 | +1273,65 (+28,97%) |

Вывод команды `pprof -top -diff_base=profiles/base.pprof profiles/result.pprof`:

```text
File: server
Type: alloc_space
Time: 2026-10-15 12:28:51 MSK
Showing nodes accounting for -1877.59MB, 28.65% of 6554.02MB total
Dropped 220 nodes (cum <= 32.77MB)
      flat  flat%   sum%        cum   cum%
 -600.26MB  9.16%  9.16% -1060.85MB 16.19%  compress/flate.NewWriter (inline)
 -375.88MB  5.74% 14.89%  -375.88MB  5.74%  compress/flate.(*fastGen).reset
 -356.38MB  5.44% 20.33%  -356.38MB  5.44%  compress/flate.(*fastGen).addBlock
 -290.93MB  4.44% 24.77%  -290.93MB  4.44%  compress/flate.newFastEnc (inline)
 -143.61MB  2.19% 26.96%  -460.59MB  7.03%  compress/flate.(*compressor).init
```
