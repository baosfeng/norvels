package acceptance

// 卷管理（C2）的断言：卷节点数、真实卷内章数、未分卷伪卷、真实卷顺序。
//
// 详情里的 volumes = 真实卷（id>0，按 idx 升序）+ 可选的「未分卷」伪卷（id=0，排在最后），
// 所以「最后一个卷」永远指向伪卷；凡指真实卷的断言都用 realVolumes()。

import (
	"fmt"
	"strings"
)

// assertVolumeNodesLastRealTitle 断言卷节点总数（want<0 表示不查数量）与最后一个真实卷的标题。
func (w *world) assertVolumeNodesLastRealTitle(want int, lastTitle string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	if len(w.structBook.Volumes) != want {
		return fmt.Errorf("详情卷节点数 = %d，期望 %d（真实卷 %d 个 + 未分卷 %v）",
			len(w.structBook.Volumes), want, len(w.realVolumes()), w.looseVolume() != nil)
	}
	return w.assertLastRealVolumeTitle(lastTitle)
}

// assertLastRealVolumeTitle 断言最后一个真实卷（按 idx 排序）的标题。
func (w *world) assertLastRealVolumeTitle(want string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	vols := w.realVolumes()
	if len(vols) == 0 {
		return fmt.Errorf("详情里没有任何真实卷")
	}
	if got := vols[len(vols)-1].Title; got != want {
		return fmt.Errorf("最后一个真实卷标题 = %q，期望 %q", got, want)
	}
	return nil
}

// assertLastRealVolumeChapters 断言最后一个真实卷内的章数。
func (w *world) assertLastRealVolumeChapters(want int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	vols := w.realVolumes()
	if len(vols) == 0 {
		return fmt.Errorf("详情里没有任何真实卷")
	}
	last := vols[len(vols)-1]
	if len(last.Chapters) != want {
		return fmt.Errorf("最后一个真实卷 %q 内章数 = %d，期望 %d", last.Title, len(last.Chapters), want)
	}
	return nil
}

// assertRealVolumeCount 断言真实卷（id>0）的数量，用于「重名卷不落库」。
func (w *world) assertRealVolumeCount(want int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	if got := len(w.realVolumes()); got != want {
		return fmt.Errorf("真实卷数 = %d，期望 %d", got, want)
	}
	return nil
}

// assertNoLooseVolume 断言详情里没有未分卷伪卷（章节已全部归卷）。
func (w *world) assertNoLooseVolume() error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	if loose := w.looseVolume(); loose != nil {
		return fmt.Errorf("详情仍有未分卷节点，内含 %d 章", len(loose.Chapters))
	}
	return nil
}

// assertOnlyLooseVolume 断言详情只剩未分卷伪卷且其内章数正确（删卷场景）。
func (w *world) assertOnlyLooseVolume(want int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	vols := w.structBook.Volumes
	if len(vols) != 1 {
		return fmt.Errorf("详情卷节点数 = %d，期望 1（只剩未分卷）", len(vols))
	}
	if vols[0].ID != 0 {
		return fmt.Errorf("唯一卷节点 id = %d，期望 0（未分卷）", vols[0].ID)
	}
	if len(vols[0].Chapters) != want {
		return fmt.Errorf("未分卷内章数 = %d，期望 %d", len(vols[0].Chapters), want)
	}
	return nil
}

// assertRealVolumeTitles 断言真实卷按 idx 排序后的标题顺序（逗号分隔）。
func (w *world) assertRealVolumeTitles(want string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	titles := make([]string, 0, len(w.structBook.Volumes))
	for _, v := range w.realVolumes() {
		titles = append(titles, v.Title)
	}
	got := strings.Join(titles, ",")
	if got != want {
		return fmt.Errorf("真实卷标题顺序 = %q，期望 %q", got, want)
	}
	return nil
}
