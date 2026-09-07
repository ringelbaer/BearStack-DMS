package de.bearstack.people.statistics

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import de.bearstack.people.data.local.Statistics

@Composable
fun StatisticsPanel(statistics: List<Statistics>) {
    Text("Nur bestätigte Aktionen zählen. Die Zahlen werden lokal für dieses Konto und diesen Datenbestand gespeichert.")
    listOf("name" to "Benannt", "assign" to "Zugeordnet", "ignore" to "Ignoriert", "skip" to "Übersprungen").forEach { (action,label) ->
        val stats = statistics.firstOrNull { it.action==action }
        Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            Text(label,style=MaterialTheme.typography.titleMedium)
            Text("Heute: ${stats?.todayFaces ?: 0} Gesichter · ${stats?.todayGroups ?: 0} Gruppen")
            Text("Insgesamt: ${stats?.faces ?: 0} Gesichter · ${stats?.groups ?: 0} Gruppen")
        } }
    }
}
