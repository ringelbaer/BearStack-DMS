package de.bearstack.people.text

import android.content.res.Resources
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalResources
import de.bearstack.people.R
import de.bearstack.people.connection.connectionDiagnostic
import java.io.IOException
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

// Keep messages language-independent in state. Resolve only when displayed so
// a locale change also translates errors that were already on screen.
data class UiText(@param:StringRes val resource: Int, val arguments: List<Any> = emptyList()) {
    constructor(@StringRes resource: Int,vararg arguments: Any):this(resource,arguments.toList())
}

class UiStrings(private val resources: Resources) {
    operator fun invoke(@StringRes resource: Int,vararg arguments: Any): String =
        resources.getString(resource,*arguments)
    operator fun invoke(message: UiText): String = invoke(message.resource,
        *message.arguments.map {if(it is UiText) invoke(it) else it}.toTypedArray())
    fun faces(count: Number): String = resources.getQuantityString(R.plurals.people_faces,if(count.toLong()==1L)1 else 2,count.toLong())
    fun groups(count: Number): String = resources.getQuantityString(R.plurals.people_groups,if(count.toLong()==1L)1 else 2,count.toLong())
    // The labeling API includes the browser's fixed German root breadcrumb.
    // Only that app label changes; source filenames and folder labels are kept.
    fun photoPath(path: String): String = if(path.startsWith("Fotos / ")) invoke(R.string.photos_title)+" / "+path.removePrefix("Fotos / ") else path
    fun date(instant: String): String = runCatching {
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM).withLocale(resources.configuration.locales[0])
            .withZone(ZoneId.systemDefault()).format(Instant.parse(instant))
    }.getOrDefault(instant)
}

@Composable fun uiStrings(): UiStrings {
    val resources=LocalResources.current
    val configuration=LocalConfiguration.current
    return remember(resources,configuration) {UiStrings(resources)}
}

interface DescribedFailure {val userText: UiText}
class UserIoFailure(override val userText: UiText): IOException("User-facing I/O failure"),DescribedFailure
class UserInputFailure(override val userText: UiText): IllegalArgumentException("Invalid input"),DescribedFailure
class UserStateFailure(override val userText: UiText): IllegalStateException("Invalid operation state"),DescribedFailure
class UserCertificateFailure(override val userText: UiText): java.security.cert.CertificateException("Certificate verification failed"),DescribedFailure

fun requireMessage(value: Boolean,@StringRes resource: Int,vararg arguments: Any) {
    if(!value) throw UserInputFailure(UiText(resource,*arguments))
}
fun checkMessage(value: Boolean,@StringRes resource: Int,vararg arguments: Any) {
    if(!value) throw UserStateFailure(UiText(resource,*arguments))
}

fun failureText(error: Throwable,pending: Boolean = false): UiText {
    // Raw exception messages can include server URLs, account names or tokens.
    generateSequence(error) {it.cause}.take(12).filterIsInstance<DescribedFailure>().firstOrNull()?.let {return it.userText}
    return connectionDiagnostic(error,pending)?.text ?: UiText(R.string.error_action)
}
