package de.bearstack.people

import android.Manifest
import de.bearstack.people.photos.*
import org.junit.Assert.*
import org.junit.Test

class DevicePhotoPermissionTest {
    @Test fun requestsOnlyPhotoPermissionsAppropriateToTheAndroidVersion() {
        assertArrayEquals(arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE), devicePhotoPermissions(26))
        assertArrayEquals(arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE), devicePhotoPermissions(32))
        assertArrayEquals(arrayOf(Manifest.permission.READ_MEDIA_IMAGES), devicePhotoPermissions(33))
        assertArrayEquals(arrayOf(Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED), devicePhotoPermissions(34))
        assertArrayEquals(devicePhotoPermissions(34), devicePhotoPermissions(36))
    }
    @Test fun fullSelectedDeniedAndRevokedAccessAreDistinguished() {
        assertEquals(DevicePhotoAccess.FULL, devicePhotoAccess(32) { it == Manifest.permission.READ_EXTERNAL_STORAGE })
        assertEquals(DevicePhotoAccess.NONE, devicePhotoAccess(33) { it == Manifest.permission.READ_EXTERNAL_STORAGE })
        assertEquals(DevicePhotoAccess.NONE, devicePhotoAccess(33) { it == Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED })
        assertEquals(DevicePhotoAccess.SELECTED, devicePhotoAccess(34) { it == Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED })
        assertEquals(DevicePhotoAccess.FULL, devicePhotoAccess(34) { true })
        assertEquals(DevicePhotoAccess.NONE, devicePhotoAccess(36) { false })
    }
}
