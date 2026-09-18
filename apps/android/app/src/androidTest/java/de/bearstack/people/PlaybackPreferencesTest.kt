package de.bearstack.people

import android.content.Context
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.photos.*
import org.junit.Assert.*
import org.junit.Test

class PlaybackPreferencesTest {
    @Test fun captionChoiceSurvivesRecreationWithoutChangingOtherPlaybackOptions() {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val store=app.getSharedPreferences("photo_playback",Context.MODE_PRIVATE)
        store.edit().clear().commit()
        try {
            val first=PlaybackPreferences(app)
            assertFalse(first.state.value.frameFolderName)
            assertFalse(first.state.value.frameRandom)
            val selected=PlaybackSettings(seconds=5,frameSeconds=30,repeat=false,frameFill=true,frameCaptions=true,frameFolderName=true,frameRandom=true)
            first.save(selected)
            assertEquals(selected,PlaybackPreferences(app).state.value)
            first.save(selected.copy(frameCaptions=false))
            assertTrue(PlaybackPreferences(app).state.value.frameFolderName)
            first.save(selected.copy(frameFolderName=false))
            assertFalse(PlaybackPreferences(app).state.value.frameFolderName)
            assertTrue(PlaybackPreferences(app).state.value.frameRandom)
        } finally {store.edit().clear().commit()}
    }
}
