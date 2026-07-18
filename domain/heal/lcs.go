package heal

// lcsLength 以 O(n*m) 时间和 O(min(n,m)) 临时空间计算两条祖先标签链之间的
// 最长公共子序列（LCS）长度。
func lcsLength(a, b []string) int {
	rows, columns := a, b
	if len(columns) > len(rows) {
		rows, columns = columns, rows
	}
	dp := make([]int, len(columns)+1)
	for _, rowValue := range rows {
		diagonal := 0
		for columnIndex, columnValue := range columns {
			above := dp[columnIndex+1]
			switch {
			case rowValue == columnValue:
				dp[columnIndex+1] = diagonal + 1
			case dp[columnIndex] > above:
				dp[columnIndex+1] = dp[columnIndex]
			}
			diagonal = above
		}
	}
	return dp[len(columns)]
}

// narrowByPathLCS 是两阶段算法的阶段一（方案 §7.2）：对每个候选的祖先路径与
// target 的路径计算 LCS 分数，只保留分数并列最大值的候选——这与
// Healenium/EPAM 所述的优化思路一致，即只对路径距离已达到观测最大值的节点，
// 才去跑（更昂贵的）启发式节点距离阶段。
func narrowByPathLCS(targetPath []string, candidates []SnapshotCandidate) []SnapshotCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	distances := make([]int, len(candidates))
	maxDist := 0
	for i, c := range candidates {
		d := lcsLength(targetPath, c.Fingerprint.Path)
		distances[i] = d
		if d > maxDist {
			maxDist = d
		}
	}
	narrowed := make([]SnapshotCandidate, 0, len(candidates))
	for i, c := range candidates {
		if distances[i] >= maxDist {
			narrowed = append(narrowed, c)
		}
	}
	return narrowed
}
