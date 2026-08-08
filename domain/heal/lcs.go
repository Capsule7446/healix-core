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

// lcsLengthWithBuffer 使用调用方提供的 dp 缓冲区计算两条路径的 LCS 长度，不保留输入引用。
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

// narrowByPathLCS 计算目标与每个候选祖先路径的 LCS 长度，只保留达到最大长度的候选，
// 以便后续评分仅处理路径相似度最高的节点；非空输入返回新的候选值切片。
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

// shortestPathLength 返回目标路径与候选路径中较短长度的最大值加一，用于分配 LCS 缓冲区。
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
