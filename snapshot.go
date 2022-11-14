package lua

type Snapshot struct {
	LPtr  *LState
	LData LState
}

func SaveSnapshot(st *LState) Snapshot {
	return Snapshot{LPtr: st, LData: *st.Clone().(*LState)}
}

func LoadSnapshot(sn Snapshot) *LState {
	*sn.LPtr = sn.LData
	return sn.LPtr
}
