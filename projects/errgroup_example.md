# errgroup 可运行示例

下面的示例展示：

- 并发执行 3 个任务
- 其中 1 个任务失败
- `errgroup.WithContext` 触发取消，其他任务通过 `ctx.Done()` 尽快退出

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"

    "golang.org/x/sync/errgroup"
)

func main() {
    ctx := context.Background()
    g, ctx := errgroup.WithContext(ctx)

    // task A：持续工作，收到取消就退出
    g.Go(func() error {
        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                fmt.Println("taskA canceled")
                return ctx.Err()
            case <-ticker.C:
                fmt.Println("taskA working")
            }
        }
    })

    // task B：1 秒后报错
    g.Go(func() error {
        time.Sleep(1 * time.Second)
        return errors.New("taskB failed")
    })

    // task C：模拟 IO/等待，也能响应取消
    g.Go(func() error {
        select {
        case <-ctx.Done():
            fmt.Println("taskC canceled")
            return ctx.Err()
        case <-time.After(5 * time.Second):
            fmt.Println("taskC done")
            return nil
        }
    })

    if err := g.Wait(); err != nil {
        fmt.Println("group error:", err)
    }
}
```

运行结果预期：

- taskA 会持续打印 working
- 1秒后 taskB 失败
- ctx 被取消，taskA/taskC 退出
- `g.Wait()` 返回 taskB 的错误

常见坑：

- goroutine 内如果阻塞在不支持 context 的调用上（例如读一个永远不返回的 channel），就算 ctx cancel 也停不下来；所以要在关键循环/阻塞点监听 `ctx.Done()`。
