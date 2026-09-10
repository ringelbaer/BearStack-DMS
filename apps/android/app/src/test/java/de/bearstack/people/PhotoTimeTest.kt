package de.bearstack.people

import de.bearstack.people.photos.photoTimeLabel
import org.junit.Assert.*
import org.junit.Test
import java.util.Locale
import java.util.TimeZone

class PhotoTimeTest {
    @Test fun sourceOffsetSurvivesDeviceTimeZoneAndBothLanguagesKeepSeconds() {
        val previous=TimeZone.getDefault()
        try {
            TimeZone.setDefault(TimeZone.getTimeZone("America/Los_Angeles"))
            assertEquals("00:15:30 · UTC+14:00",photoTimeLabel("2024-01-02T00:15:30+14:00",Locale.GERMAN))
            // CLDR uses a narrow no-break space before AM on newer JDKs.
            assertEquals("12:15:30 AM · UTC+14:00",photoTimeLabel("2024-01-02T00:15:30+14:00",Locale.US)?.replace('\u202f',' '))
            assertEquals("10:00:00 · UTC",photoTimeLabel("2026-09-09T10:00:00Z",Locale.GERMAN))
            assertNull(photoTimeLabel("unknown",Locale.GERMAN))
        } finally {TimeZone.setDefault(previous)}
    }
}
