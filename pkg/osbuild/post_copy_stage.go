package osbuild

type PostCopyStageOptions struct {
	Sysroot string `json:"sysroot"`
}

func (PostCopyStageOptions) isStageOptions() {}

func NewPostCopyStage(mounts *Mounts) *Stage {
	return &Stage{
		Type: "org.osbuild.ostree.post-copy",
		Options: &PostCopyStageOptions{
			Sysroot: "/",
		},
		Mounts: *mounts,
	}
}
