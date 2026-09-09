package de.bearstack.people.statistics

import de.bearstack.people.text.uiStrings
import de.bearstack.people.R
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import de.bearstack.people.data.local.Statistics

@Composable
fun StatisticsPanel(statistics: List<Statistics>) {
    val text=uiStrings()
    Text(text(R.string.statistics_help))
    listOf("name" to text(R.string.statistics_named), "assign" to text(R.string.statistics_assigned), "ignore" to text(R.string.statistics_ignored), "skip" to text(R.string.statistics_skipped)).forEach { (action,label) ->
        val stats = statistics.firstOrNull { it.action==action }
        Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            Text(label,style=MaterialTheme.typography.titleMedium)
            Text(text(R.string.statistics_today,text.faces(stats?.todayFaces ?: 0),text.groups(stats?.todayGroups ?: 0)))
            Text(text(R.string.statistics_total,text.faces(stats?.faces ?: 0),text.groups(stats?.groups ?: 0)))
        } }
    }
}
