package acceptance

// 卷管理（C2）与章节跨卷移动的步骤：新建 / 改名 / 删除 / 重排 + move。
// 关键不变量：结构操作的失败请求不落库，章数在移动前后保持一致。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

// registerVolumeSteps 注册卷管理相关步骤。
func registerVolumeSteps(sc *godog.ScenarioContext, w *world) {
	registerVolumeActionSteps(sc, w)
	registerVolumeAssertSteps(sc, w)
}

// registerVolumeActionSteps 注册会改库的卷动作。
func registerVolumeActionSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我新建一个名为 "(.+)" 的卷$`, func(ctx context.Context, title string) error {
		return w.createVolume(title, http.StatusCreated)
	})
	sc.Step(`^我已新建一个名为 "(.+)" 的卷$`, func(ctx context.Context, title string) error {
		return w.createVolume(title, http.StatusCreated)
	})
	sc.Step(`^我再新建一个名为 "(.+)" 的卷$`, func(ctx context.Context, title string) error {
		return w.postVolumeRaw(title)
	})
	sc.Step(`^我把这个卷改名为 "(.+)"$`, func(ctx context.Context, title string) error {
		return w.renameLastVolume(title)
	})
	sc.Step("^我把本书全部章节移到这个卷$", func(ctx context.Context) error {
		return w.moveAllChaptersToLastVolume()
	})
	sc.Step(`^我已把第 (\d+) 章移到第 (\d+) 个真实卷$`, func(ctx context.Context, n, k string) error {
		return w.moveChapterToRealVolume(atoiSafe(n), atoiSafe(k))
	})
	sc.Step("^我删除这个卷$", func(ctx context.Context) error {
		return w.deleteLastVolume()
	})
	sc.Step("^我把最后一个卷排到最前$", func(ctx context.Context) error {
		return w.reorderRealVolumesDesc()
	})
}

// registerVolumeAssertSteps 注册卷结构断言。
func registerVolumeAssertSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^新建卷接口应返回 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertLastCode(atoiSafe(want))
	})
	sc.Step(`^书籍详情应有 (\d+) 个卷节点，最后一个真实卷的标题为 "(.+)"$`,
		func(ctx context.Context, n, title string) error {
			return w.assertVolumeNodesLastRealTitle(atoiSafe(n), title)
		})
	sc.Step(`^书籍详情最后一个真实卷内应有 (\d+) 章$`, func(ctx context.Context, want string) error {
		return w.assertLastRealVolumeChapters(atoiSafe(want))
	})
	sc.Step(`^书籍真实的卷数应为 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertRealVolumeCount(atoiSafe(want))
	})
	sc.Step(`^书籍详情最后一个真实卷的标题应为 "(.+)"$`, func(ctx context.Context, want string) error {
		return w.assertLastRealVolumeTitle(want)
	})
	sc.Step(`^移动接口应返回 (\d+) 章$`, func(ctx context.Context, want string) error {
		return w.assertLastIntField("moved", atoiSafe(want))
	})
	sc.Step("^书籍详情不应再有未分卷节点$", func(ctx context.Context) error {
		return w.assertNoLooseVolume()
	})
	sc.Step(`^删除卷接口应返回 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertLastCode(atoiSafe(want))
	})
	sc.Step(`^书籍详情应只剩未分卷节点，其内 (\d+) 章$`, func(ctx context.Context, want string) error {
		return w.assertOnlyLooseVolume(atoiSafe(want))
	})
	sc.Step(`^重排卷序接口应返回 (\d+)$`, func(ctx context.Context, want string) error {
		return w.assertLastCode(atoiSafe(want))
	})
	sc.Step(`^书籍详情里真实卷的标题顺序应为 "(.+)"$`, func(ctx context.Context, want string) error {
		return w.assertRealVolumeTitles(want)
	})
}

// createVolume 新建卷并校验 201 + 回包里的卷节点；记下卷 id 供后续步骤引用。
func (w *world) createVolume(title string, want int) error {
	if err := w.checkBook(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/volumes", w.lastBookID), string(body))
	if err != nil {
		return err
	}
	if code != want {
		return fmt.Errorf("POST 新建卷 %q 状态码 = %d，期望 %d，body=%s", title, code, want, truncate(string(data), 200))
	}
	return w.captureVolume(data, title)
}

// postVolumeRaw 只发一次新建卷请求，状态码留给断言步骤检查（重名场景）。
func (w *world) postVolumeRaw(title string) error {
	if err := w.checkBook(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	_, _, err = w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/volumes", w.lastBookID), string(body))
	return err
}

// captureVolume 解析 201 回包并记下新卷 id。
func (w *world) captureVolume(data []byte, title string) error {
	var res struct {
		Volume volumeDTO `json:"volume"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("新建卷响应不是预期 JSON: %w (body=%s)", err, truncate(string(data), 200))
	}
	if res.Volume.ID <= 0 {
		return fmt.Errorf("新建卷返回的 volume.id = %d，期望正整数", res.Volume.ID)
	}
	if res.Volume.Title != title {
		return fmt.Errorf("新建卷标题 = %q，期望 %q", res.Volume.Title, title)
	}
	w.lastVolumeID = res.Volume.ID
	return nil
}

// renameLastVolume 改最近新建卷的名字：200 + 卷节点。
func (w *world) renameLastVolume(title string) error {
	if err := w.requireVolume(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPatch,
		fmt.Sprintf("/api/books/%d/volumes/%d", w.lastBookID, w.lastVolumeID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PATCH 卷名状态码 = %d，期望 200，body=%s", code, truncate(string(data), 200))
	}
	return nil
}

// deleteLastVolume 删最近新建的卷：204，卷内章节由服务端落回未分卷。
func (w *world) deleteLastVolume() error {
	if err := w.requireVolume(); err != nil {
		return err
	}
	_, _, err := w.doJSON(http.MethodDelete,
		fmt.Sprintf("/api/books/%d/volumes/%d", w.lastBookID, w.lastVolumeID), "")
	return err
}

// moveAllChaptersToLastVolume 把本书全部章节移到最近新建的卷。
func (w *world) moveAllChaptersToLastVolume() error {
	if err := w.requireVolume(); err != nil {
		return err
	}
	if err := w.refreshStructure(); err != nil {
		return err
	}
	return w.moveChapters(idsOf(w.structChapters), w.lastVolumeID)
}

// moveChapterToRealVolume 把第 n 章移到第 k 个真实卷（1 起，按 idx 排序）。
func (w *world) moveChapterToRealVolume(n, k int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	vols := w.realVolumes()
	if k < 1 || k > len(vols) {
		return fmt.Errorf("第 %d 个真实卷不存在（当前共 %d 个）", k, len(vols))
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	return w.moveChapters([]int64{ch.ID}, vols[k-1].ID)
}

// moveChapters 调 move 接口，校验 200 且 moved 与请求章数一致。
func (w *world) moveChapters(ids []int64, volumeID int64) error {
	body, err := json.Marshal(map[string]any{"chapterIds": ids, "volumeId": volumeID})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost,
		fmt.Sprintf("/api/books/%d/chapters/move", w.lastBookID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("POST 移动章节状态码 = %d，期望 200，body=%s", code, truncate(string(data), 200))
	}
	var res struct {
		Moved int `json:"moved"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("移动响应不是预期 JSON: %w (body=%s)", err, truncate(string(data), 200))
	}
	if res.Moved != len(ids) {
		return fmt.Errorf("移动接口返回 moved = %d，期望 %d（请求 %d 章）", res.Moved, len(ids), len(ids))
	}
	return nil
}

// reorderRealVolumesDesc 把真实卷的现有顺序反过来重排。
func (w *world) reorderRealVolumesDesc() error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	vols := w.realVolumes()
	if len(vols) < 2 {
		return fmt.Errorf("真实卷数 = %d，无法重排", len(vols))
	}
	ids := make([]int64, 0, len(vols))
	for i := len(vols) - 1; i >= 0; i-- {
		ids = append(ids, vols[i].ID)
	}
	body, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPut,
		fmt.Sprintf("/api/books/%d/volumes/order", w.lastBookID), string(body))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("PUT 卷序状态码 = %d，期望 200，body=%s", code, truncate(string(data), 200))
	}
	return nil
}

// checkBook 确认本书已导入。
func (w *world) checkBook() error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	return nil
}

// requireVolume 确认最近新建过卷。
func (w *world) requireVolume() error {
	if err := w.checkBook(); err != nil {
		return err
	}
	if w.lastVolumeID <= 0 {
		return fmt.Errorf("尚未新建卷")
	}
	return nil
}
