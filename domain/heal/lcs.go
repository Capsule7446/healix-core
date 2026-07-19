package heal

// lcsLength 以 O(n*m) 时间和 O(min(n,m)) 临时空间计算两条祖先标签链之间的
// 最长公共子序列（LCS）长度。
func lcsLength(a, b []string) int {
	rows, columns := a, b
	if len(columns) > len(rows) {
		rows, columns = columns, rows
	}
	dp := make([]int, len(columns)+1)
	return lcsLengthWithBuffer(rows, columns, dp)
}

func lcsLengthWithBuffer(rows, columns []string, dp []int) int {
	clear(dp)
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

// narrowByPathLCS 是候选筛选算法：对每个候选的祖先路径与
// target 的路径计算 LCS 分数，只保留分数并列最大值的候选——这与
// Healenium/EPAM 所述的优化思路一致，即只对路径距离已达到观测最大值的节点，
// 才去跑（更昂贵的）启发式节点距离阶段。
func narrowByPathLCS(targetPath []string, candidates []SnapshotCandidate) []SnapshotCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	distances := make([]int, len(candidates))
	maxDist := 0
	workspace := make([]int, shortestPathLength(targetPath, candidates))
	for i, c := range candidates {
		rows, columns := targetPath, c.Fingerprint.Path
		if len(columns) > len(rows) {
			rows, columns = columns, rows
		}
		d := lcsLengthWithBuffer(rows, columns, workspace[:len(columns)+1])
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

func shortestPathLength(targetPath []string, candidates []SnapshotCandidate) int {
	length := 0
	for _, candidate := range candidates {
		candidateLength := len(candidate.Fingerprint.Path)
		if len(targetPath) < candidateLength {
			candidateLength = len(targetPath)
		}
		if candidateLength > length {
			length = candidateLength
		}
	}
	return length + 1
}
