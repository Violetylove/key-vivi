package platform

// SID解析未导出，同包验证截断边界，并在竞态检查下实际读取进程令牌。
import "testing"

func TestIntegrityRID(t *testing.T) {
	valid := []byte{1, 1, 0, 0, 0, 0, 0, 16, 0, 32, 0, 0}
	for n := 0; n < len(valid); n++ {
		if _, err := integrityRID(valid[:n]); err == nil {
			t.Fatalf("accepted truncated SID of length %d", n)
		}
	}
	for _, index := range []int{0, 1, 2, 7} {
		bad := append([]byte(nil), valid...)
		bad[index] = 0xff
		if _, err := integrityRID(bad); err == nil {
			t.Fatalf("accepted invalid SID field %d", index)
		}
	}
	if got, err := integrityRID(valid); err != nil || got != IntegrityMedium {
		t.Fatal("medium SID", got, err)
	}
	multiple := append([]byte(nil), valid...)
	multiple[1] = 2
	multiple = append(multiple, 0, 48, 0, 0)
	if got, err := integrityRID(multiple); err != nil || got != IntegrityHigh {
		t.Fatal("last sub-authority", got, err)
	}
}

func TestIntegrityLevelReadsCurrentToken(t *testing.T) {
	level, err := IntegrityLevel()
	if err != nil {
		t.Fatal(err)
	}
	restricted, sameLevel, err := RestrictedIntegrity()
	if err != nil || sameLevel != level || restricted != (level < IntegrityMedium) {
		t.Fatal("inconsistent token classification", restricted, level, sameLevel, err)
	}
	t.Logf("current token integrity=%#x", level)
}
