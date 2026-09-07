package de.bearstack.people

import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.connection.Profile
import de.bearstack.people.connection.ProfileStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import java.io.File

class ProfileStoreTest {
    @Test fun credentialsSurviveEncryptedStorageAndNeverAppearInTheFile() = runBlocking {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val store=ProfileStore(context)
        try {
            val profile=Profile("https://example.invalid/","tester","secret-not-in-file")
            store.write(profile)
            assertEquals(profile,ProfileStore(context).read())
            val encoded=File(context.noBackupFilesDir,"connection.enc").readBytes().toString(Charsets.ISO_8859_1)
            assertFalse(encoded.contains(profile.password));assertFalse(encoded.contains(profile.url))
        } finally {store.clear()}
        assertNull(store.read())
    }
}
