/*
Copyright 2020 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package oauth

import (
	"net/http"

	"kubesphere.io/kubesphere/pkg/apiserver/authentication"

	"github.com/emicklei/go-restful"
	restfulspec "github.com/emicklei/go-restful-openapi"

	"kubesphere.io/kubesphere/pkg/api"
	"kubesphere.io/kubesphere/pkg/apiserver/authentication/oauth"
	"kubesphere.io/kubesphere/pkg/constants"
	"kubesphere.io/kubesphere/pkg/models/auth"
	"kubesphere.io/kubesphere/pkg/models/iam/im"
)

const contentTypeFormData = "application/x-www-form-urlencoded"

// AddToContainer ks-apiserver includes a built-in OAuth server. Users obtain OAuth access tokens to authenticate themselves to the API.
// The OAuth server supports standard authorization code grant and the implicit grant OAuth authorization flows.
// All requests for OAuth tokens involve a request to <ks-apiserver>/oauth/authorize.
// Most authentication integrations place an authenticating proxy in front of this endpoint, or configure ks-apiserver
// to validate credentials against a backing identity provider.
// Requests to <ks-apiserver>/oauth/authorize can come from user-agents that cannot display interactive login pages, such as the CLI.
func AddToContainer(c *restful.Container, im im.IdentityManagementInterface,
	tokenOperator auth.TokenManagementInterface,
	passwordAuthenticator auth.PasswordAuthenticator,
	oauth2Authenticator auth.OAuthAuthenticator,
	loginRecorder auth.LoginRecorder,
	options *authentication.Options) error {

	ws := &restful.WebService{}
	ws.Path("/oauth").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	handler := newHandler(im, tokenOperator, passwordAuthenticator, oauth2Authenticator, loginRecorder, options)

	ws.Route(ws.GET("/.well-known/openid-configuration").To(handler.discovery).
		Doc("The OpenID Provider's configuration information can be retrieved."))
	ws.Route(ws.GET("/keys").To(handler.keys).
		Doc("OP's JSON Web Key Set [JWK] document."))
	ws.Route(ws.GET("/userinfo").To(handler.userinfo).
		Doc("UserInfo Endpoint is an OAuth 2.0 Protected Resource that returns Claims about the authenticated End-User."))

	// Implement webhook authentication interface
	// https://kubernetes.io/docs/reference/access-authn-authz/authentication/#webhook-token-authentication
	ws.Route(ws.POST("/authenticate").
		Doc("TokenReview attempts to authenticate a token to a known user. Note: TokenReview requests may be "+
			"cached by the webhook token authenticator plugin in the kube-apiserver.").
		Reads(TokenReview{}).
		To(handler.tokenReview).
		Returns(http.StatusOK, api.StatusOK, TokenReview{}).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	// https://datatracker.ietf.org/doc/html/rfc6749#section-3.1
	ws.Route(ws.GET("/authorize").
		Doc("The authorization endpoint is used to interact with the resource owner and obtain an authorization grant.").
		Param(ws.QueryParameter("response_type", "The value MUST be one of \"code\" for requesting an "+
			"authorization code as described by [RFC6749] Section 4.1.1, \"token\" for requesting an access token (implicit grant)"+
			" as described by [RFC6749] Section 4.2.2.").Required(true)).
		Param(ws.QueryParameter("client_id", "OAuth 2.0 Client Identifier valid at the Authorization Server.").Required(true)).
		Param(ws.QueryParameter("redirect_uri", "Redirection URI to which the response will be sent. "+
			"This URI MUST exactly match one of the Redirection URI values for the Client pre-registered at the OpenID Provider.").Required(true)).
		Param(ws.QueryParameter("scope", "OpenID Connect requests MUST contain the openid scope value. "+
			"If the openid scope value is not present, the behavior is entirely unspecified.").Required(false)).
		Param(ws.QueryParameter("state", "Opaque value used to maintain state between the request and the callback.").Required(false)).
		To(handler.authorize).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	// Authorization Servers MUST support the use of the HTTP GET and POST methods
	// defined in RFC 2616 [RFC2616] at the Authorization Endpoint.
	ws.Route(ws.POST("/authorize").
		Consumes(contentTypeFormData).
		Doc("The authorization endpoint is used to interact with the resource owner and obtain an authorization grant.").
		Param(ws.BodyParameter("response_type", "The value MUST be one of \"code\" for requesting an "+
			"authorization code as described by [RFC6749] Section 4.1.1, \"token\" for requesting an access token (implicit grant)"+
			" as described by [RFC6749] Section 4.2.2.").Required(true)).
		Param(ws.BodyParameter("client_id", "OAuth 2.0 Client Identifier valid at the Authorization Server.").Required(true)).
		Param(ws.BodyParameter("redirect_uri", "Redirection URI to which the response will be sent. "+
			"This URI MUST exactly match one of the Redirection URI values for the Client pre-registered at the OpenID Provider.").Required(true)).
		Param(ws.BodyParameter("scope", "OpenID Connect requests MUST contain the openid scope value. "+
			"If the openid scope value is not present, the behavior is entirely unspecified.").Required(false)).
		Param(ws.BodyParameter("state", "Opaque value used to maintain state between the request and the callback.").Required(false)).
		To(handler.authorize).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	// https://datatracker.ietf.org/doc/html/rfc6749#section-3.2
	ws.Route(ws.POST("/token").
		Consumes(contentTypeFormData).
		Doc("The resource owner password credentials grant type is suitable in\n"+
			"cases where the resource owner has a trust relationship with the\n"+
			"client, such as the device operating system or a highly privileged application.").
		Param(ws.FormParameter("grant_type", "OAuth defines four grant types: "+
			"authorization code, implicit, resource owner password credentials, and client credentials.").
			Required(true)).
		Param(ws.FormParameter("client_id", "Valid client credential.").Required(true)).
		Param(ws.FormParameter("client_secret", "Valid client credential.").Required(true)).
		Param(ws.FormParameter("username", "The resource owner username.").Required(false)).
		Param(ws.FormParameter("password", "The resource owner password.").Required(false)).
		Param(ws.FormParameter("code", "Valid authorization code.").Required(false)).
		To(handler.token).
		Returns(http.StatusOK, http.StatusText(http.StatusOK), &oauth.Token{}).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	// Authorization callback URL, where the end of the URL contains the identity provider name.
	// The provider name is also used to build the callback URL.
	ws.Route(ws.GET("/callback/{callback}").
		Doc("OAuth callback API, the path param callback is config by identity provider").
		Param(ws.QueryParameter("access_token", "The access token issued by the authorization server.").
			Required(true)).
		Param(ws.QueryParameter("token_type", "The type of the token issued as described in [RFC6479] Section 7.1. "+
			"Value is case insensitive.").Required(true)).
		Param(ws.QueryParameter("expires_in", "The lifetime in seconds of the access token.  For "+
			"example, the value \"3600\" denotes that the access token will "+
			"expire in one hour from the time the response was generated."+
			"If omitted, the authorization server SHOULD provide the "+
			"expiration time via other means or document the default value.")).
		Param(ws.QueryParameter("scope", "if identical to the scope requested by the client;"+
			"otherwise, REQUIRED.  The scope of the access token as described by [RFC6479] Section 3.3.").Required(false)).
		Param(ws.QueryParameter("state", "if the \"state\" parameter was present in the client authorization request."+
			"The exact value received from the client.").Required(true)).
		To(handler.oauthCallback).
		Returns(http.StatusOK, api.StatusOK, oauth.Token{}).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	// https://openid.net/specs/openid-connect-rpinitiated-1_0.html
	ws.Route(ws.GET("/logout").
		Doc("This endpoint takes an ID token and logs the user out of KubeSphere if the "+
			"subject matches the current session.").
		Param(ws.QueryParameter("id_token_hint", "ID Token previously issued by the OP "+
			"to the RP passed to the Logout Endpoint as a hint about the End-User's current authenticated "+
			"session with the Client. This is used as an indication of the identity of the End-User that "+
			"the RP is requesting be logged out by the OP.").Required(false)).
		Param(ws.QueryParameter("post_logout_redirect_uri", "URL to which the RP is requesting "+
			"that the End-User's User Agent be redirected after a logout has been performed. ").Required(false)).
		Param(ws.QueryParameter("state", "Opaque value used by the RP to maintain state between "+
			"the logout request and the callback to the endpoint specified by the post_logout_redirect_uri parameter.").
			Required(false)).
		To(handler.logout).
		Returns(http.StatusOK, http.StatusText(http.StatusOK), "").
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	ws.Route(ws.POST("/login/{identityprovider}").
		Consumes(contentTypeFormData).
		Doc("Login by identity provider user").
		Param(ws.PathParameter("identityprovider", "The identity provider name")).
		Param(ws.FormParameter("username", "The username of the relevant user in ldap")).
		Param(ws.FormParameter("password", "The password of the relevant user in ldap")).
		To(handler.loginByIdentityProvider).
		Returns(http.StatusOK, http.StatusText(http.StatusOK), oauth.Token{}).
		Metadata(restfulspec.KeyOpenAPITags, []string{constants.AuthenticationTag}))

	c.Add(ws)

	return nil
}
